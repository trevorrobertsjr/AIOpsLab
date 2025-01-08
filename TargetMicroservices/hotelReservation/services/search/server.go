package search

import (
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/harlow/go-micro-services/dialer"
	"github.com/harlow/go-micro-services/registry"
	geo "github.com/harlow/go-micro-services/services/geo/proto"
	rate "github.com/harlow/go-micro-services/services/rate/proto"
	pb "github.com/harlow/go-micro-services/services/search/proto"
	"github.com/harlow/go-micro-services/tls"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/stats"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const name = "srv-search"

// tracerStatsHandler implements gRPC stats.Handler for Datadog tracing.
type tracerStatsHandler struct{}

func (t *tracerStatsHandler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	span, ctx := tracer.StartSpanFromContext(ctx, info.FullMethodName, tracer.ResourceName("grpc"))
	return tracer.ContextWithSpan(ctx, span)
}

func (t *tracerStatsHandler) HandleRPC(ctx context.Context, rpcStats stats.RPCStats) {
	span, ok := tracer.SpanFromContext(ctx)
	if !ok {
		return
	}

	// Handle specific RPC stats events
	switch statsEvent := rpcStats.(type) {
	case *stats.InPayload:
		span.SetTag("event", "in_payload")
		span.SetTag("bytes_received", statsEvent.Length)
	case *stats.OutPayload:
		span.SetTag("event", "out_payload")
		span.SetTag("bytes_sent", statsEvent.Length)
	case *stats.End:
		if statsEvent.Error != nil {
			span.SetTag(ext.Error, statsEvent.Error)
		}
	default:
		span.SetTag("event", "unknown")
	}

	span.Finish()
}

func (t *tracerStatsHandler) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}

func (t *tracerStatsHandler) HandleConn(context.Context, stats.ConnStats) {}

// Server implements the search service
type Server struct {
	geoClient  geo.GeoClient
	rateClient rate.RateClient
	Port       int
	IpAddr     string
	KnativeDns string
	Registry   *registry.Client
	uuid       string
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
		grpc.StatsHandler(&tracerStatsHandler{}),
	}

	if tlsopt := tls.GetServerOpt(); tlsopt != nil {
		opts = append(opts, tlsopt)
	}

	srv := grpc.NewServer(opts...)
	pb.RegisterSearchServer(srv, s)

	// Initialize gRPC clients
	if err := s.initGeoClient("srv-geo"); err != nil {
		return err
	}
	if err := s.initRateClient("srv-rate"); err != nil {
		return err
	}

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
		return fmt.Errorf("Failed to register: %v", err)
	}
	log.Info().Msg("Successfully registered in consul")

	return srv.Serve(lis)
}

// Shutdown cleans up any processes
func (s *Server) Shutdown() {
	s.Registry.Deregister(s.uuid)
}

func (s *Server) initGeoClient(name string) error {
	conn, err := s.getGrpcConn(name)
	if err != nil {
		return fmt.Errorf("Dialer error: %v", err)
	}
	s.geoClient = geo.NewGeoClient(conn)
	return nil
}

func (s *Server) initRateClient(name string) error {
	conn, err := s.getGrpcConn(name)
	if err != nil {
		return fmt.Errorf("Dialer error: %v", err)
	}
	s.rateClient = rate.NewRateClient(conn)
	return nil
}

func (s *Server) getGrpcConn(name string) (*grpc.ClientConn, error) {
	// If Knative DNS is configured, use it for the service address
	if s.KnativeDns != "" {
		target := fmt.Sprintf("%s.%s", name, s.KnativeDns)
		log.Info().Msgf("Dialing Knative DNS target: %s", target)
		return dialer.Dial(target, dialer.WithTracer())
	}

	// Default to Consul-based service discovery
	target := fmt.Sprintf("consul:///%s", name)
	log.Info().Msgf("Dialing Consul target: %s at %s", target, s.Registry.Address())
	return dialer.Dial(target, dialer.WithTracer())
}

// Nearby returns IDs of nearby hotels ordered by ranking algo
func (s *Server) Nearby(ctx context.Context, req *pb.NearbyRequest) (*pb.SearchResult, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "search.Nearby", tracer.ResourceName("Nearby"))
	defer span.Finish()

	log.Trace().Msg("In Search Nearby")

	nearby, err := s.geoClient.Nearby(ctx, &geo.Request{
		Lat: req.Lat,
		Lon: req.Lon,
	})
	if err != nil {
		span.SetTag(ext.Error, true)
		return nil, err
	}

	// Find rates for hotels
	rates, err := s.rateClient.GetRates(ctx, &rate.Request{
		HotelIds: nearby.HotelIds,
		InDate:   req.InDate,
		OutDate:  req.OutDate,
	})
	if err != nil {
		span.SetTag(ext.Error, true)
		return nil, err
	}

	// Build the response
	res := new(pb.SearchResult)
	for _, ratePlan := range rates.RatePlans {
		res.HotelIds = append(res.HotelIds, ratePlan.HotelId)
	}
	return res, nil
}
