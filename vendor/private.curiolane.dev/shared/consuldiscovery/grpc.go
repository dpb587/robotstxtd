package consuldiscovery

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc/resolver"
)

// GRPCScheme is the resolver scheme used for gRPC dial targets returned by
// NewGRPCTarget for a Consul-backed service. It is registered globally with
// grpc-go's resolver registry the first time NewGRPCTarget creates such a
// target.
const GRPCScheme = "consuldiscovery"

// GRPCServiceConfigJSON is a grpc.WithDefaultServiceConfig payload that
// pairs with GRPCScheme targets. It enables the "round_robin" load-balancing
// policy, which is required for a grpc.NewClient to actually open (and
// spread RPCs across) connections to every address a GRPCScheme target
// resolves to, plus a retry policy so a single RPC that fails against one
// instance (before any response is received) is transparently retried
// against another already-connected instance rather than surfacing the
// failure to the caller.
const GRPCServiceConfigJSON = `{
	"loadBalancingConfig": [{"round_robin": {}}],
	"methodConfig": [{
		"name": [{}],
		"retryPolicy": {
			"maxAttempts": 4,
			"initialBackoff": "0.1s",
			"maxBackoff": "1s",
			"backoffMultiplier": 2,
			"retryableStatusCodes": ["UNAVAILABLE"]
		}
	}]
}`

var (
	grpcResolverOnce sync.Once
	grpcRegistryMu   sync.Mutex
	grpcRegistry     = map[string]*Watcher{}
)

// NewGRPCTarget returns a gRPC dial target for raw, for use with
// grpc.NewClient. If raw uses the Consul DNS service naming convention (see
// ParseHostname), it registers a Watcher (appended to *watchers, for the
// caller to Close on shutdown) with a grpc-go resolver.Builder under
// GRPCScheme, so that grpc.NewClient(target, grpc.WithDefaultServiceConfig(
// GRPCServiceConfigJSON)) maintains connections to every healthy Consul
// catalog instance simultaneously and updates them as they change. This
// lets a single RPC fail over to a different, already-connected instance
// without waiting on that one persistent connection to notice a failure and
// redial. Otherwise raw is returned unchanged, for grpc.NewClient's own
// default resolution.
func NewGRPCTarget(consulSvc *Service, log *slog.Logger, refreshInterval time.Duration, raw string, watchers *[]*Watcher) (string, error) {
	serviceName, ok := ParseHostname(ExtractHost(raw))
	if !ok {
		return raw, nil
	}

	if consulSvc == nil {
		return "", fmt.Errorf("%q is a consul service hostname but consulServiceDiscovery is not configured", raw)
	}

	watcher := consulSvc.Watch(log, serviceName, refreshInterval)
	*watchers = append(*watchers, watcher)

	grpcResolverOnce.Do(func() {
		resolver.Register(&grpcResolverBuilder{})
	})

	grpcRegistryMu.Lock()
	grpcRegistry[serviceName] = watcher
	grpcRegistryMu.Unlock()

	return GRPCScheme + ":///" + serviceName, nil
}

type grpcResolverBuilder struct{}

func (*grpcResolverBuilder) Scheme() string { return GRPCScheme }

func (*grpcResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, _ resolver.BuildOptions) (resolver.Resolver, error) {
	serviceName := target.Endpoint()

	grpcRegistryMu.Lock()
	watcher, ok := grpcRegistry[serviceName]
	grpcRegistryMu.Unlock()

	if !ok {
		return nil, fmt.Errorf("consuldiscovery: no watcher registered for service %q", serviceName)
	}

	r := &grpcResolver{cc: cc, watcher: watcher}
	r.push()
	r.unsubscribe = watcher.subscribe(r.push)

	return r, nil
}

type grpcResolver struct {
	cc          resolver.ClientConn
	watcher     *Watcher
	unsubscribe func()
}

func (r *grpcResolver) push() {
	addrs := r.watcher.Addresses()
	state := resolver.State{Addresses: make([]resolver.Address, len(addrs))}

	for i, addr := range addrs {
		state.Addresses[i] = resolver.Address{Addr: addr}
	}

	r.cc.UpdateState(state)
}

func (r *grpcResolver) ResolveNow(resolver.ResolveNowOptions) { r.push() }

func (r *grpcResolver) Close() { r.unsubscribe() }
