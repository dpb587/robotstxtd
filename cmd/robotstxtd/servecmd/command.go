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

	httpcachedclient "github.com/dpb587/httpcached/client"
	"github.com/dpb587/robotstxtd/cmd/robotstxtdpflag"
	"github.com/dpb587/robotstxtd/robotstxtdpb"
	"github.com/dpb587/robotstxtd/server"
	"github.com/dpb587/robotstxtd/server/sqlite3"
	"github.com/spf13/cobra"
	"golang.org/x/net/http/httpproxy"
	"google.golang.org/grpc"
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
	var bot = &robotstxtdpflag.UserAgent{}

	var (
		httpClient     *http.Client
		httpClientErr  error
		httpClientOnce sync.Once
	)

	httpClientFactory := func() (*http.Client, error) {
		httpClientOnce.Do(func() {
			httpClient, httpClientErr = func() (*http.Client, error) {
				if len(httpcachedServer) > 0 {
					serverURL, err := url.Parse(httpcachedServer)
					if err != nil {
						return nil, fmt.Errorf("httpcached-server: parse: %v", err)
					}

					serverTransport := &http.Transport{
						DialContext: (&net.Dialer{
							Timeout:   16 * time.Second,
							KeepAlive: 8 * time.Second,
						}).DialContext,
						DisableCompression:    true,
						MaxConnsPerHost:       32,
						MaxIdleConns:          64,
						IdleConnTimeout:       90 * time.Second,
						TLSHandshakeTimeout:   10 * time.Second,
						ExpectContinueTimeout: 1 * time.Second,
					}

					blobTransport := &http.Transport{
						DialContext: (&net.Dialer{
							Timeout:   16 * time.Second,
							KeepAlive: 8 * time.Second,
						}).DialContext,
						DisableCompression:    true,
						MaxConnsPerHost:       32,
						MaxIdleConns:          64,
						IdleConnTimeout:       90 * time.Second,
						TLSHandshakeTimeout:   10 * time.Second,
						ExpectContinueTimeout: 1 * time.Second,
					}

					return &http.Client{
						Transport: httpcachedclient.NewService(log, serverTransport, serverURL, blobTransport, httpcachedTier),
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

	return cmd
}
