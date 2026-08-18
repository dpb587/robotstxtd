package consuldiscovery

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	consulapi "github.com/hashicorp/consul/api"
)

type instance struct {
	address string
	port    int
}

type Watcher struct {
	client      *consulapi.Client
	serviceName string
	log         *slog.Logger

	mu          sync.Mutex
	instances   []instance
	noInstances bool
	next        uint64
	subscribers map[int]func()
	nextSubID   int

	stopCh chan struct{}
	wg     sync.WaitGroup
}

func (s *Service) Watch(log *slog.Logger, serviceName string, refreshInterval time.Duration) *Watcher {
	if refreshInterval <= 0 {
		refreshInterval = DefaultRefreshInterval
	}

	w := &Watcher{
		client:      s.client,
		serviceName: serviceName,
		log:         log,
		stopCh:      make(chan struct{}),
	}

	w.refresh()

	w.wg.Go(func() {

		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				w.refresh()
			case <-w.stopCh:
				return
			}
		}
	})

	return w
}

func (w *Watcher) refresh() {
	entries, _, err := w.client.Health().Service(w.serviceName, "", true, nil)
	if err != nil {
		w.log.Warn("consul service refresh failed", "service", w.serviceName, "error", err)

		return
	}

	instances := make([]instance, 0, len(entries))

	for _, entry := range entries {
		address := entry.Service.Address
		if address == "" {
			address = entry.Node.Address
		}

		instances = append(instances, instance{
			address: address,
			port:    entry.Service.Port,
		})
	}

	rand.Shuffle(len(instances), func(i, j int) {
		instances[i], instances[j] = instances[j], instances[i]
	})

	w.mu.Lock()
	w.instances = instances
	writeLog := (len(instances) == 0 && !w.noInstances) || (len(instances) > 0 && w.noInstances)
	w.noInstances = len(instances) == 0
	subscribers := make([]func(), 0, len(w.subscribers))
	for _, fn := range w.subscribers {
		subscribers = append(subscribers, fn)
	}
	w.mu.Unlock()

	if writeLog {
		if len(instances) == 0 {
			w.log.Warn("consul service has no healthy instances", "service", w.serviceName)
		} else {
			w.log.Info("consul service has healthy instances", "service", w.serviceName)
		}
	}

	for _, fn := range subscribers {
		fn()
	}
}

// Addresses returns "host:port" for every healthy instance known as of the
// most recent refresh, in the current randomized round-robin order.
func (w *Watcher) Addresses() []string {
	w.mu.Lock()
	instances := w.instances
	w.mu.Unlock()

	addrs := make([]string, len(instances))
	for i, inst := range instances {
		addrs[i] = net.JoinHostPort(inst.address, strconv.Itoa(inst.port))
	}

	return addrs
}

// subscribe registers fn to be called (with no arguments) after every
// refresh, including refreshes that leave the instance set unchanged. The
// returned unsubscribe function removes fn; it is safe to call more than
// once.
func (w *Watcher) subscribe(fn func()) (unsubscribe func()) {
	w.mu.Lock()
	if w.subscribers == nil {
		w.subscribers = make(map[int]func())
	}
	id := w.nextSubID
	w.nextSubID++
	w.subscribers[id] = fn
	w.mu.Unlock()

	return func() {
		w.mu.Lock()
		delete(w.subscribers, id)
		w.mu.Unlock()
	}
}

// Pick returns the next "host:port" from the set of healthy instances known
// as of the most recent refresh, walking round-robin through a randomized
// ordering that is reshuffled on each refresh.
func (w *Watcher) Pick() (string, error) {
	w.mu.Lock()
	instances := w.instances
	w.mu.Unlock()

	if len(instances) == 0 {
		return "", fmt.Errorf("consul: %s: no healthy instances available", w.serviceName)
	}

	idx := atomic.AddUint64(&w.next, 1) % uint64(len(instances))
	picked := instances[idx]

	return net.JoinHostPort(picked.address, strconv.Itoa(picked.port)), nil
}

// DialContext adapts Pick into a dial function usable as the DialContext for
// http.Transport, grpc.WithContextDialer, or net.Resolver.Dial. It ignores
// the address it is given; the actual peer is always the current Consul pick.
func (w *Watcher) DialContext(ctx context.Context, network, _ string) (net.Conn, error) {
	addr, err := w.Pick()
	if err != nil {
		return nil, err
	}

	return (&net.Dialer{
		Timeout:   16 * time.Second,
		KeepAlive: 8 * time.Second,
	}).DialContext(ctx, network, addr)
}

func (w *Watcher) Close() {
	close(w.stopCh)
	w.wg.Wait()
}
