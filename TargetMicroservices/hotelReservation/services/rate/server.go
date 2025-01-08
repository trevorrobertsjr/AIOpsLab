package rate

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/google/uuid"
	"github.com/harlow/go-micro-services/registry"
	pb "github.com/harlow/go-micro-services/services/rate/proto"
	"github.com/harlow/go-micro-services/tls"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const name = "srv-rate"

// Server implements the rate service
type Server struct {
	Port         int
	IpAddr       string
	MongoSession *mgo.Session
	Registry     *registry.Client
	MemcClient   *memcache.Client
	uuid         string
}

// Run starts the server
func (s *Server) Run() error {
	if s.Port == 0 {
		return fmt.Errorf("server port must be set")
	}

	s.uuid = uuid.New().String()

	opts := []grpc.ServerOption{
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Timeout: 120 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			PermitWithoutStream: true,
		}),
	}

	if tlsopt := tls.GetServerOpt(); tlsopt != nil {
		opts = append(opts, tlsopt)
	}

	srv := grpc.NewServer(opts...)
	pb.RegisterRateServer(srv, s)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.Port))
	if err != nil {
		log.Fatal().Msgf("Failed to listen: %v", err)
	}

	// Construct service DNS address without the prefix
	namespace := "test-hotel-reservation"                                     // Replace with your namespace
	serviceDNS := fmt.Sprintf("%s.%s.svc.cluster.local", name[4:], namespace) // Strip "srv-" from `name`

	log.Info().Msgf("Registering service [name: %s, id: %s, address: %s, port: %d]", name, s.uuid, serviceDNS, s.Port)
	err = s.Registry.Register(name, s.uuid, serviceDNS, s.Port)
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

// GetRates gets rates for hotels for a specific date range.
func (s *Server) GetRates(ctx context.Context, req *pb.Request) (*pb.Result, error) {
	log.Trace().Msg("In GetRates")

	res := new(pb.Result)
	ratePlans := make(RatePlans, 0)

	hotelIds := []string{}
	rateMap := make(map[string]struct{})
	for _, hotelID := range req.HotelIds {
		hotelIds = append(hotelIds, hotelID)
		rateMap[hotelID] = struct{}{}
	}

	memSpan, _ := tracer.StartSpanFromContext(ctx, "memcached.get_multi_rate", tracer.Tag(ext.SpanType, "cache"))
	memSpan.SetTag(ext.Component, "memcached")
	memSpan.SetTag(ext.PeerService, "memcached-rate")
	resMap, err := s.MemcClient.GetMulti(hotelIds)
	memSpan.Finish()

	var wg sync.WaitGroup
	var mutex sync.Mutex
	if err != nil && err != memcache.ErrCacheMiss {
		log.Panic().Msgf("Memcached error while trying to get hotel [id: %v]: %v", hotelIds, err)
	} else {
		for hotelId, item := range resMap {
			rateStrs := strings.Split(string(item.Value), "\n")
			log.Trace().Msgf("Memcached hit, hotelId = %s, rate strings: %v", hotelId, rateStrs)

			for _, rateStr := range rateStrs {
				if len(rateStr) != 0 {
					rateP := new(pb.RatePlan)
					json.Unmarshal([]byte(rateStr), rateP)
					ratePlans = append(ratePlans, rateP)
				}
			}
			delete(rateMap, hotelId)
		}
		wg.Add(len(rateMap))
		for hotelId := range rateMap {
			go func(id string) {
				defer wg.Done()
				session := s.MongoSession.Copy()
				defer session.Close()
				c := session.DB("rate-db").C("inventory")

				tmpRatePlans := make(RatePlans, 0)
				mongoSpan, _ := tracer.StartSpanFromContext(ctx, "mongo.query", tracer.Tag(ext.SpanType, "db"))
				mongoSpan.SetTag(ext.DBInstance, "rate-db")
				mongoSpan.SetTag(ext.DBStatement, fmt.Sprintf("Find rates for hotel ID: %s", id))
				err := c.Find(&bson.M{"hotelId": id}).All(&tmpRatePlans)
				mongoSpan.Finish()

				if err != nil {
					log.Panic().Msgf("Failed to find rates for hotel ID [%v]: %v", id, err)
					return
				}
				memcStr := ""
				for _, r := range tmpRatePlans {
					mutex.Lock()
					ratePlans = append(ratePlans, r)
					mutex.Unlock()
					rateJson, _ := json.Marshal(r)
					memcStr += string(rateJson) + "\n"
				}
				s.MemcClient.Set(&memcache.Item{Key: id, Value: []byte(memcStr)})
			}(hotelId)
		}
	}
	wg.Wait()

	sort.Sort(ratePlans)
	res.RatePlans = ratePlans

	return res, nil
}

// RatePlans implements sorting for rate plans
type RatePlans []*pb.RatePlan

func (r RatePlans) Len() int {
	return len(r)
}

func (r RatePlans) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}

func (r RatePlans) Less(i, j int) bool {
	return r[i].RoomType.TotalRate > r[j].RoomType.TotalRate
}
