package configset

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/dpb587/httpcached/client"
)

type Client struct {
	Tier   string `yaml:"tier"`
	Server string `yaml:"server"`

	// Dialer optionally overrides how connections to Server are established
	// (e.g. to resolve the host through a service discovery mechanism other
	// than DNS). When nil, a default net.Dialer is used.
	Dialer func(ctx context.Context, network, addr string) (net.Conn, error) `yaml:"-"`
}

func (csc *Client) New(log *slog.Logger) (*client.Service, error) {
	serverURL, err := url.Parse(csc.Server)
	if err != nil {
		return nil, fmt.Errorf("server: parse: %v", err)
	}

	dialContext := csc.Dialer
	if dialContext == nil {
		dialContext = (&net.Dialer{
			Timeout:   16 * time.Second,
			KeepAlive: 8 * time.Second,
		}).DialContext
	}

	return client.NewService(
		log,
		&http.Transport{
			DialContext:           dialContext,
			DisableCompression:    true,
			MaxConnsPerHost:       32,
			MaxIdleConns:          64,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		serverURL,
		&http.Transport{
			DialContext:           dialContext,
			DisableCompression:    true,
			MaxConnsPerHost:       32,
			MaxIdleConns:          64,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		csc.Tier,
	), nil
}
