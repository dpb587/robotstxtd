package servecmd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/dpb587/robotstxtd/cmd/robotstxtdpflag"
	"github.com/dpb587/robotstxtd/inspector"
	"github.com/dpb587/robotstxtd/robotstxtdpb"
	"github.com/dpb587/robotstxtd/server"
	"github.com/dpb587/robotstxtd/server/sqlite3"
	"github.com/spf13/cobra"
	"golang.org/x/net/proxy"
	"google.golang.org/grpc"
)

func NewCommand() *cobra.Command {
	log := slog.Default()

	var bindAddress string = "127.0.0.1:0"
	var sqlite3DSN string
	var caCert string
	var socks5Proxy string
	var inspectorBind string
	var bot = &robotstxtdpflag.UserAgent{}

	cmd := &cobra.Command{
		Use: "serve",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := sqlite3.Open(sqlite3DSN)
			if err != nil {
				return fmt.Errorf("open db: %v", err)
			}

			httpClient, err := newHTTPClient(caCert, socks5Proxy)
			if err != nil {
				return fmt.Errorf("http client: %v", err)
			}

			if socks5Proxy != "" {
				host, port, err := socks5Endpoint(socks5Proxy)
				if err != nil {
					return fmt.Errorf("http client: %v", err)
				}

				log.Info(
					"socks5 proxy",
					"host", host,
					"port", port,
				)
			}

			srv := server.NewService(log, db, httpClient, 30*time.Second, bot.NewProto())
			defer srv.Close()

			go func() {
				tt := time.NewTicker(12 * time.Hour)

				for {
					<-tt.C

					err := srv.Prune()
					if err != nil {
						log.Error(
							"prune",
							"error", err,
						)
					}
				}
			}()

			if inspectorBind != "" {
				handler, err := inspector.NewHandler(srv)
				if err != nil {
					return fmt.Errorf("inspector: %v", err)
				}

				inspectorLn, err := net.Listen("tcp", inspectorBind)
				if err != nil {
					return fmt.Errorf("inspector bind: %v", err)
				}

				log.Info(
					"inspector",
					"bind", inspectorLn.Addr().String(),
				)

				go func() {
					err := http.Serve(inspectorLn, handler)
					if err != nil && !errors.Is(err, http.ErrServerClosed) {
						log.Error(
							"inspector stopped",
							"error", err,
						)
					}
				}()
			}

			lis, err := net.Listen("tcp", bindAddress)
			if err != nil {
				return fmt.Errorf("bind: %v", err)
			}

			log.Info(
				"listening",
				"bind", lis.Addr().String(),
			)

			grpcServer := grpc.NewServer()
			robotstxtdpb.RegisterCrawlerServer(grpcServer, srv)

			return grpcServer.Serve(lis)
		},
	}

	cmd.Flags().StringVar(&sqlite3DSN, "db-sqlite3-dsn", sqlite3DSN, "")
	cmd.Flags().StringVar(&bindAddress, "bind", bindAddress, "")
	bot.BindFlags(cmd.Flags(), "bot-")
	cmd.Flags().StringVar(&caCert, "ca-cert", caCert, "PEM file of upstream TLS roots")
	cmd.Flags().StringVar(&socks5Proxy, "socks5-proxy", socks5Proxy, "optional SOCKS5 proxy (socks5://[user:password@]host:port); empty dials directly")
	cmd.Flags().StringVar(&inspectorBind, "inspector-bind", inspectorBind, "optional unauthenticated inspector address, for example 127.0.0.1:9079")

	return cmd
}

func newHTTPClient(caCert, socks5Proxy string) (*http.Client, error) {
	baseDialer := &net.Dialer{
		Timeout: 16 * time.Second,
	}

	var dialContext func(ctx context.Context, network, addr string) (net.Conn, error)

	if socks5Proxy != "" {
		proxyURL, err := parseSOCKS5Proxy(socks5Proxy)
		if err != nil {
			return nil, err
		}

		// A new dial to the proxy host:port on every request. Keep-alives are
		// disabled below, and net.Dialer resolves that name each time so a DNS
		// name can round-robin. The destination is passed through as a SOCKS5
		// hostname; upstream names are not resolved locally.
		dialer, err := proxy.FromURL(proxyURL, baseDialer)
		if err != nil {
			return nil, errors.New("socks5-proxy: dialer")
		}

		if cd, ok := dialer.(proxy.ContextDialer); ok {
			dialContext = cd.DialContext
		} else {
			dialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			}
		}
	} else {
		dialContext = baseDialer.DialContext
	}

	var tlsConfig *tls.Config

	if len(caCert) > 0 {
		certPool := x509.NewCertPool()

		caCertBytes, err := os.ReadFile(caCert)
		if err != nil {
			return nil, fmt.Errorf("ca-cert: %v", err)
		}

		for len(caCertBytes) > 0 {
			var block *pem.Block
			block, caCertBytes = pem.Decode(caCertBytes)
			if block == nil {
				break
			} else if block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
				return nil, fmt.Errorf("ca-cert: invalid block: %s", block.Type)
			}

			certBytes := block.Bytes
			cert, err := x509.ParseCertificate(certBytes)
			if err != nil {
				return nil, fmt.Errorf("ca-cert: parse certificate: %v", err)
			}

			certPool.AddCert(cert)
		}

		tlsConfig = &tls.Config{
			RootCAs: certPool,
		}
	}

	transport := &http.Transport{
		DialContext:           dialContext,
		DisableKeepAlives:     true,
		ResponseHeaderTimeout: 8 * time.Second,
		TLSClientConfig:       tlsConfig,
		TLSHandshakeTimeout:   16 * time.Second,
		TLSNextProto:          map[string]func(authority string, c *tls.Conn) http.RoundTripper{},
	}

	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("maximum redirects followed")
			}

			return nil
		},
		Timeout:   32 * time.Second,
		Transport: transport,
	}, nil
}

func parseSOCKS5Proxy(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "socks5" || u.Hostname() == "" || u.Port() == "" {
		return nil, errors.New("socks5-proxy: expected socks5://[user:password@]host:port")
	}

	return u, nil
}

func socks5Endpoint(raw string) (host, port string, err error) {
	u, err := parseSOCKS5Proxy(raw)
	if err != nil {
		return "", "", err
	}

	return u.Hostname(), u.Port(), nil
}
