package servecmd

import (
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
	"sync"
	"time"

	"github.com/dpb587/httpcached/client/configset"
	"github.com/dpb587/robotstxtd/cmd/robotstxtdpflag"
	"github.com/dpb587/robotstxtd/robotstxtdpb"
	"github.com/dpb587/robotstxtd/server"
	"github.com/dpb587/robotstxtd/server/sqlite3"
	"github.com/spf13/cobra"
	"golang.org/x/net/http/httpproxy"
	"google.golang.org/grpc"
	"private.curiolane.dev/shared/consuldiscovery"
)

func NewCommand() *cobra.Command {
	log := slog.Default()

	var bindAddress string = "127.0.0.1:0"
	var sqlite3DSN string
	var httpProxy string
	var httpsProxy string
	var caCert string
	var httpcachedServer string
	var httpcachedTier string
	var consulAddress string
	var consulScheme string
	var consulDatacenter string
	var consulToken string
	var consulRefreshInterval = consuldiscovery.DefaultRefreshInterval
	var bot = &robotstxtdpflag.UserAgent{}

	var consulWatchers []*consuldiscovery.Watcher

	var (
		httpClient     *http.Client
		httpClientErr  error
		httpClientOnce sync.Once
	)

	httpClientFactory := func() (*http.Client, error) {
		httpClientOnce.Do(func() {
			httpClient, httpClientErr = func() (*http.Client, error) {
				if len(httpcachedServer) > 0 {
					var consulSvc *consuldiscovery.Service

					if len(consulAddress) > 0 {
						var err error

						consulSvc, err = consuldiscovery.NewService(consuldiscovery.Config{
							Address:         consulAddress,
							Scheme:          consulScheme,
							Datacenter:      consulDatacenter,
							Token:           consulToken,
							RefreshInterval: consulRefreshInterval,
						})
						if err != nil {
							return nil, fmt.Errorf("consul service discovery: %v", err)
						}
					}

					dialer, err := consuldiscovery.NewDialer(consulSvc, log, consulRefreshInterval, httpcachedServer, &consulWatchers)
					if err != nil {
						return nil, fmt.Errorf("httpcached-server: %v", err)
					}

					httpcachedClient := &configset.Client{
						Tier:   httpcachedTier,
						Server: httpcachedServer,
						Dialer: dialer,
					}

					service, err := httpcachedClient.New(log)
					if err != nil {
						return nil, fmt.Errorf("httpcached-server: %v", err)
					}

					return &http.Client{
						Transport: service,
					}, nil
				}

				var proxyFunc func(*url.URL) (*url.URL, error)

				if len(httpProxy) > 0 || len(httpsProxy) > 0 {
					cfg := &httpproxy.Config{
						HTTPProxy:  httpProxy,
						HTTPSProxy: httpsProxy,
					}

					proxyFunc = cfg.ProxyFunc()
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
					Dial: (&net.Dialer{
						Timeout: 16 * time.Second,
					}).Dial,
					DisableKeepAlives:     true,
					ResponseHeaderTimeout: 8 * time.Second,
					TLSClientConfig:       tlsConfig,
					TLSHandshakeTimeout:   16 * time.Second,
				}

				if proxyFunc != nil {
					transport.Proxy = func(r *http.Request) (*url.URL, error) {
						return proxyFunc(r.URL)
					}
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
			}()
		})

		return httpClient, httpClientErr
	}

	cmd := &cobra.Command{
		Use: "serve",
		RunE: func(cmd *cobra.Command, args []string) error {
			lis, err := net.Listen("tcp", bindAddress)
			if err != nil {
				return fmt.Errorf("bind: %v", err)
			}

			log.Info(
				"listening",
				"bind", lis.Addr().String(),
			)

			defer func() {
				for _, w := range consulWatchers {
					w.Close()
				}
			}()

			grpcServer := grpc.NewServer()

			{
				db, err := sqlite3.Open(sqlite3DSN)
				if err != nil {
					return fmt.Errorf("open db: %v", err)
				}

				httpClient, err := httpClientFactory()
				if err != nil {
					return fmt.Errorf("http client: %v", err)
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

				robotstxtdpb.RegisterCrawlerServer(grpcServer, srv)
			}

			return grpcServer.Serve(lis)
		},
	}

	cmd.Flags().StringVar(&sqlite3DSN, "db-sqlite3-dsn", sqlite3DSN, "")
	cmd.Flags().StringVar(&bindAddress, "bind", bindAddress, "")
	bot.BindFlags(cmd.Flags(), "bot-")
	cmd.Flags().StringVar(&httpProxy, "http-proxy", httpProxy, "")
	cmd.Flags().StringVar(&httpsProxy, "https-proxy", httpsProxy, "")
	cmd.Flags().StringVar(&caCert, "ca-cert", caCert, "")
	cmd.Flags().StringVar(&httpcachedServer, "httpcached-server", httpcachedServer, "")
	cmd.Flags().StringVar(&httpcachedTier, "httpcached-tier", httpcachedTier, "")
	cmd.Flags().StringVar(&consulAddress, "consul-address", consulAddress, "")
	cmd.Flags().StringVar(&consulScheme, "consul-scheme", consulScheme, "")
	cmd.Flags().StringVar(&consulDatacenter, "consul-datacenter", consulDatacenter, "")
	cmd.Flags().StringVar(&consulToken, "consul-token", consulToken, "")
	cmd.Flags().DurationVar(&consulRefreshInterval, "consul-refresh-interval", consulRefreshInterval, "")

	return cmd
}
