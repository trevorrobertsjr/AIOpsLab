package profile

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/google/uuid"
	"github.com/harlow/go-micro-services/registry"
	pb "github.com/harlow/go-micro-services/services/profile/proto"
	"github.com/harlow/go-micro-services/tls"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/stats"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const name = "srv-profile"

// tracerStatsHandler implements gRPC stats.Handler for Datadog tracing.
type tracerStatsHandler struct{}

func (t *tracerStatsHandler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	span, ctx := tracer.StartSpanFromContext(ctx, info.FullMethodName, tracer.SpanType(ext.SpanTypeRPC))
	return tracer.ContextWithSpan(ctx, span)
}

func (t *tracerStatsHandler) HandleRPC(ctx context.Context, stats stats.RPCStats) {
	span := tracer.SpanFromContext(ctx)
	if span == nil {
		return
	}
	if errStats, ok := stats.(*stats.End); ok && errStats.Error != nil {
		span.SetTag(ext.Error, errStats.Error)
	}
	span.Finish()
}

func (t *tracerStatsHandler) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}

func (t *tracerStatsHandler) HandleConn(context.Context, stats.ConnStats) {}

// Server implements the profile service
type Server struct {
	uuid         string
	Port         int
	IpAddr       string
	MongoSession *mgo.Session
	Registry     *registry.Client
	MemcClient   *memcache.Client
}

// Run starts the server
func (s *Server) Run() error {
	if s.Port == 0 {
		return fmt.Errorf("server port must be set")
	}

	s.uuid = uuid.New().String()

	log.Trace().Msgf("Starting profile service at %s:%d", s.IpAddr, s.Port)

	opts := []grpc.ServerOption{
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Timeout: 120 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			PermitWithoutStream: true,
		}),
		grpc.StatsHandler(&tracerStatsHandler{}), // Datadog tracing
	}

	if tlsopt := tls.GetServerOpt(); tlsopt != nil {
		opts = append(opts, tlsopt)
	}

	srv := grpc.NewServer(opts...)
	pb.RegisterProfileServer(srv, s)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.Port))
	if err != nil {
		log.Fatal().Msgf("Failed to configure listener: %v", err)
	}

	err = s.Registry.Register(name, s.uuid, s.IpAddr, s.Port)
	if err != nil {
		return fmt.Errorf("failed to register: %v", err)
	}
	log.Info().Msg("Successfully registered in consul")

	return srv.Serve(lis)
}

// Shutdown cleans up any processes
func (s *Server) Shutdown() {
	s.Registry.Deregister(s.uuid)
}

// GetProfiles returns hotel profiles for requested IDs
func (s *Server) GetProfiles(ctx context.Context, req *pb.Request) (*pb.Result, error) {
	log.Trace().Msg("In GetProfiles")

	res := new(pb.Result)
	hotels := make([]*pb.Hotel, 0)
	var wg sync.WaitGroup
	var mutex sync.Mutex

	hotelIds := make([]string, 0)
	profileMap := make(map[string]struct{})
	for _, hotelId := range req.HotelIds {
		hotelIds = append(hotelIds, hotelId)
		profileMap[hotelId] = struct{}{}
	}

	memSpan, ctx := tracer.StartSpanFromContext(ctx, "memcached.get_profile", tracer.SpanType(ext.SpanTypeCache))
	memSpan.SetTag(ext.Component, "memcached")
	memSpan.SetTag(ext.PeerService, "memcached-profile")
	resMap, err := s.MemcClient.GetMulti(hotelIds)
	memSpan.Finish()
	if err != nil && err != memcache.ErrCacheMiss {
		log.Panic().Msgf("Memcached error: %v", err)
	}

	for hotelId, item := range resMap {
		hotelProf := new(pb.Hotel)
		json.Unmarshal(item.Value, hotelProf)
		hotels = append(hotels, hotelProf)
		delete(profileMap, hotelId)
	}

	wg.Add(len(profileMap))
	for hotelId := range profileMap {
		go func(hotelId string) {
			defer wg.Done()
			session := s.MongoSession.Copy()
			defer session.Close()
			c := session.DB("profile-db").C("hotels")

			hotelProf := new(pb.Hotel)
			mongoSpan, ctx := tracer.StartSpanFromContext(ctx, "mongo.query", tracer.SpanType(ext.SpanTypeDB))
			mongoSpan.SetTag(ext.DBInstance, "profile-db")
			mongoSpan.SetTag(ext.DBStatement, fmt.Sprintf("Find hotel by ID: %s", hotelId))
			err := c.Find(bson.M{"id": hotelId}).One(&hotelProf)
			mongoSpan.Finish()

			if err != nil {
				log.Error().Msgf("Failed to get hotel data: %v", err)
				return
			}

			mutex.Lock()
			hotels = append(hotels, hotelProf)
			mutex.Unlock()

			// Cache result in Memcached
			profJson, _ := json.Marshal(hotelProf)
			s.MemcClient.Set(&memcache.Item{Key: hotelId, Value: profJson})
		}(hotelId)
	}
	wg.Wait()

	res.Hotels = hotels
	return res, nil
}
