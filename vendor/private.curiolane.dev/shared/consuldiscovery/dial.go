package consuldiscovery

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"time"
)

// Picker returns the next dialable "host:port" for a resolved target.
type Picker interface {
	Pick() (string, error)
}

// staticPicker is a Picker that always returns the same literal address.
type staticPicker string

func (p staticPicker) Pick() (string, error) {
	return string(p), nil
}

// NewPicker returns a Picker for raw. If raw uses the Consul DNS service
// naming convention (see ParseHostname), a Watcher is created (and appended
// to *watchers, for the caller to Close on shutdown) that round-robins
// across the service's healthy Consul catalog instances. Otherwise raw is
// treated as a literal "host:port" and returned unchanged on every Pick().
func NewPicker(consulSvc *Service, log *slog.Logger, refreshInterval time.Duration, raw string, watchers *[]*Watcher) (Picker, error) {
	serviceName, ok := ParseHostname(ExtractHost(raw))
	if !ok {
		return staticPicker(raw), nil
	}

	if consulSvc == nil {
		return nil, fmt.Errorf("%q is a consul service hostname but consulServiceDiscovery is not configured", raw)
	}

	watcher := consulSvc.Watch(log, serviceName, refreshInterval)
	*watchers = append(*watchers, watcher)

	return watcher, nil
}

// NewDialer returns a dial function backed by a Consul-discovered service
// when raw refers to a Consul DNS service hostname (see ParseHostname). It
// returns a nil dialer (and no error) when raw is a literal address, so the
// caller can fall back to its own default dialer.
func NewDialer(consulSvc *Service, log *slog.Logger, refreshInterval time.Duration, raw string, watchers *[]*Watcher) (func(ctx context.Context, network, addr string) (net.Conn, error), error) {
	serviceName, ok := ParseHostname(ExtractHost(raw))
	if !ok {
		return nil, nil
	}

	if consulSvc == nil {
		return nil, fmt.Errorf("%q is a consul service hostname but consulServiceDiscovery is not configured", raw)
	}

	watcher := consulSvc.Watch(log, serviceName, refreshInterval)
	*watchers = append(*watchers, watcher)

	return watcher.DialContext, nil
}

// ExtractHost pulls the bare hostname out of raw, which may be a URL (e.g.
// "http://host:port/"), a "host:port" pair, or a bare hostname.
func ExtractHost(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Hostname()
	}

	if host, _, err := net.SplitHostPort(raw); err == nil {
		return host
	}

	return raw
}
