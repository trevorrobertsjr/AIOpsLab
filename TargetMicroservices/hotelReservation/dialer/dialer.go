package dialer

import (
	"context"
	"fmt"
	"time"

	"github.com/harlow/go-micro-services/tls"
	"github.com/hashicorp/consul/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/stats"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// consulResolverBuilder handles building resolvers for Consul service discovery.
type consulResolverBuilder struct {
	client *api.Client
}

// Build constructs the resolver for the given target.
func (b *consulResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	r := &consulResolver{
		client: b.client,
		// Correctly retrieve the string value of the target's Endpoint
		target: target.Endpoint(), // Call the function to get the string value
		cc:     cc,
		stopCh: make(chan struct{}),
	}
	go r.watchConsul()
	return r, nil
}

// Scheme returns the scheme supported by this resolver builder.
func (b *consulResolverBuilder) Scheme() string {
	return "consul"
}

// consulResolver watches for updates to the Consul service and updates the gRPC resolver state.
type consulResolver struct {
	client *api.Client
	target string
	cc     resolver.ClientConn
	stopCh chan struct{}
}

// watchConsul continuously fetches service updates from Consul and updates the resolver state.
func (r *consulResolver) watchConsul() {
	for {
		select {
		case <-r.stopCh:
			return
		default:
			services, _, err := r.client.Health().Service(r.target, "", true, nil)
			if err != nil {
				fmt.Printf("Failed to resolve services: %v\n", err)
				time.Sleep(5 * time.Second)
				continue
			}

			addresses := make([]resolver.Address, 0, len(services))
			for _, service := range services {
				addr := service.Service.Address
				if addr == "" {
					addr = service.Node.Address
				}
				addresses = append(addresses, resolver.Address{
					Addr: fmt.Sprintf("%s:%d", addr, service.Service.Port),
				})
			}
			r.cc.UpdateState(resolver.State{Addresses: addresses})
			time.Sleep(5 * time.Second)
		}
	}
}

// ResolveNow is a no-op for this resolver.
func (r *consulResolver) ResolveNow(options resolver.ResolveNowOptions) {}

// Close stops the resolver.
func (r *consulResolver) Close() {
	close(r.stopCh)
}

// tracerStatsHandler implements gRPC stats.Handler for Datadog tracing.
type tracerStatsHandler struct{}

// TagRPC creates a new span for RPC calls and attaches it to the context.
func (t *tracerStatsHandler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	span, ctx := tracer.StartSpanFromContext(ctx, info.FullMethodName, tracer.Tag(ext.SpanType, "rpc"))
	return tracer.ContextWithSpan(ctx, span)
}

// HandleRPC handles RPC events and adds relevant tags to the span.
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
	case *stats.InHeader:
		span.SetTag("event", "in_header")
	case *stats.OutHeader:
		span.SetTag("event", "out_header")
	case *stats.End:
		if statsEvent.Error != nil {
			span.SetTag(ext.Error, statsEvent.Error)
		}
	default:
		span.SetTag("event", "unknown")
	}

	span.Finish()
}

// TagConn creates a span for connection-level events.
func (t *tracerStatsHandler) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}

// HandleConn handles connection-level events (currently a no-op).
func (t *tracerStatsHandler) HandleConn(context.Context, stats.ConnStats) {}

// WithTracer enables Datadog tracing for gRPC calls.
func WithTracer() DialOption {
	return func(name string) (grpc.DialOption, error) {
		return grpc.WithStatsHandler(&tracerStatsHandler{}), nil
	}
}

// DialOption allows optional configurations for gRPC Dial.
type DialOption func(name string) (grpc.DialOption, error)

// Dial creates a gRPC connection to the target service.
func Dial(target string, opts ...DialOption) (*grpc.ClientConn, error) {
	dialopts := []grpc.DialOption{
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Timeout:             120 * time.Second,
			PermitWithoutStream: true,
		}),
	}

	// Use TLS if available
	if tlsopt := tls.GetDialOpt(); tlsopt != nil {
		dialopts = append(dialopts, tlsopt)
	} else {
		dialopts = append(dialopts, grpc.WithInsecure())
	}

	for _, opt := range opts {
		o, err := opt(target)
		if err != nil {
			return nil, err
		}
		dialopts = append(dialopts, o)
	}

	resolver.Register(&consulResolverBuilder{
		client: getConsulClient(),
	})

	conn, err := grpc.Dial(fmt.Sprintf("consul:///%s", target), dialopts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to service %s: %v", target, err)
	}

	return conn, nil
}

// getConsulClient initializes a new Consul client.
func getConsulClient() *api.Client {
	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		panic(fmt.Sprintf("failed to create Consul client: %v", err))
	}
	return client
}
