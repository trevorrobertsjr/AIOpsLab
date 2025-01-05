package recommendation

import (
	"fmt"
	"math"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/hailocab/go-geoindex"
	"github.com/harlow/go-micro-services/registry"
	pb "github.com/harlow/go-micro-services/services/recommendation/proto"
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

const name = "srv-recommendation"

// Server implements the recommendation service
type Server struct {
	hotels       map[string]Hotel
	Port         int
	IpAddr       string
	MongoSession *mgo.Session
	Registry     *registry.Client
	uuid         string
}

// Run starts the server
func (s *Server) Run() error {
	if s.Port == 0 {
		return fmt.Errorf("server port must be set")
	}

	if s.hotels == nil {
		s.hotels = loadRecommendations(s.MongoSession)
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

	pb.RegisterRecommendationServer(srv, s)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.Port))
	if err != nil {
		log.Fatal().Msgf("Failed to listen: %v", err)
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

// GetRecommendations returns recommendations within a given requirement.
func (s *Server) GetRecommendations(ctx context.Context, req *pb.Request) (*pb.Result, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "recommendation.get_recommendations", tracer.ResourceName("GetRecommendations"))
	defer span.Finish()

	res := new(pb.Result)
	require := req.Require

	if require == "dis" {
		p1 := &geoindex.GeoPoint{Plat: req.Lat, Plon: req.Lon}
		min := math.MaxFloat64
		for _, hotel := range s.hotels {
			tmp := float64(geoindex.Distance(p1, &geoindex.GeoPoint{Plat: hotel.HLat, Plon: hotel.HLon})) / 1000
			if tmp < min {
				min = tmp
			}
		}
		for _, hotel := range s.hotels {
			tmp := float64(geoindex.Distance(p1, &geoindex.GeoPoint{Plat: hotel.HLat, Plon: hotel.HLon})) / 1000
			if tmp == min {
				res.HotelIds = append(res.HotelIds, hotel.HId)
			}
		}
	} else if require == "rate" {
		max := 0.0
		for _, hotel := range s.hotels {
			if hotel.HRate > max {
				max = hotel.HRate
			}
		}
		for _, hotel := range s.hotels {
			if hotel.HRate == max {
				res.HotelIds = append(res.HotelIds, hotel.HId)
			}
		}
	} else if require == "price" {
		min := math.MaxFloat64
		for _, hotel := range s.hotels {
			if hotel.HPrice < min {
				min = hotel.HPrice
			}
		}
		for _, hotel := range s.hotels {
			if hotel.HPrice == min {
				res.HotelIds = append(res.HotelIds, hotel.HId)
			}
		}
	} else {
		log.Warn().Msgf("Invalid 'require' parameter: %v", require)
		span.SetTag(ext.Error, true)
	}

	return res, nil
}

// loadRecommendations loads hotel recommendations from MongoDB.
func loadRecommendations(session *mgo.Session) map[string]Hotel {
	s := session.Copy()
	defer s.Close()

	c := s.DB("recommendation-db").C("recommendation")

	var hotels []Hotel
	err := c.Find(bson.M{}).All(&hotels)
	if err != nil {
		log.Error().Msgf("Failed to get hotel data: %v", err)
	}

	profiles := make(map[string]Hotel)
	for _, hotel := range hotels {
		profiles[hotel.HId] = hotel
	}

	return profiles
}

// Hotel struct represents a hotel recommendation.
type Hotel struct {
	ID     bson.ObjectId `bson:"_id"`
	HId    string        `bson:"hotelId"`
	HLat   float64       `bson:"lat"`
	HLon   float64       `bson:"lon"`
	HRate  float64       `bson:"rate"`
	HPrice float64       `bson:"price"`
}
