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
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

type consulResolverBuilder struct {
	client *api.Client
}

func (b *consulResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	updates := make(chan []*resolver.Address, 1)
	r := &consulResolver{
		client:  b.client,
		target:  target,
		cc:      cc,
		updates: updates,
		stopCh:  make(chan struct{}),
	}
	go r.watchConsul()
	return r, nil
}

func (b *consulResolverBuilder) Scheme() string {
	return "consul"
}

type consulResolver struct {
	client  *api.Client
	target  resolver.Target
	cc      resolver.ClientConn
	updates chan []*resolver.Address
	stopCh  chan struct{}
}

func (r *consulResolver) watchConsul() {
	for {
		select {
		case <-r.stopCh:
			return
		default:
			services, _, err := r.client.Health().Service(r.target.Endpoint, "", true, nil)
			if err != nil {
				fmt.Printf("Failed to resolve services: %v\n", err)
				time.Sleep(5 * time.Second)
				continue
			}

			var addresses []*resolver.Address
			for _, service := range services {
				addr := service.Service.Address
				if addr == "" {
					addr = service.Node.Address
				}
				addresses = append(addresses, &resolver.Address{
					Addr: fmt.Sprintf("%s:%d", addr, service.Service.Port),
				})
			}
			r.cc.UpdateState(resolver.State{Addresses: addresses})
			time.Sleep(5 * time.Second)
		}
	}
}

func (r *consulResolver) ResolveNow(options resolver.ResolveNowOptions) {}

func (r *consulResolver) Close() {
	close(r.stopCh)
}

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

// WithTracer enables Datadog tracing for gRPC calls
func WithTracer() DialOption {
	return func(name string) (grpc.DialOption, error) {
		return grpc.WithStatsHandler(&tracerStatsHandler{}), nil
	}
}

// Dial creates a gRPC connection to the target service
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

// getConsulClient initializes a new Consul client
func getConsulClient() *api.Client {
	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		panic(fmt.Sprintf("failed to create Consul client: %v", err))
	}
	return client
}
