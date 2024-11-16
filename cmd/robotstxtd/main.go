package main

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

	"github.com/dpb587/robotstxtd/api"
	"github.com/dpb587/robotstxtd/server"
	"github.com/dpb587/robotstxtd/server/storage"
	"github.com/spf13/cobra"
	"golang.org/x/net/http/httpproxy"
	"google.golang.org/grpc"
)

func main() {
	log := slog.Default()

	var bindAddress string = "127.0.0.1:0"
	var httpProxy string = ""
	var httpsProxy string = ""
	var caCert string = ""
	var bot = &api.Bot{}

	var (
		httpClient     *http.Client
		httpClientErr  error
		httpClientOnce sync.Once
	)

	httpClientFactory := func() (*http.Client, error) {
		httpClientOnce.Do(func() {
			httpClient, httpClientErr = func() (*http.Client, error) {
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

				return &http.Client{
					CheckRedirect: func(req *http.Request, via []*http.Request) error {
						if len(via) >= 5 {
							return errors.New("maximum redirects followed")
						}

						return nil
					},
					Timeout: 32 * time.Second,
					Transport: &http.Transport{
						Dial: (&net.Dialer{
							Timeout: 16 * time.Second,
						}).Dial,
						DisableKeepAlives: true,
						Proxy: func(r *http.Request) (*url.URL, error) {
							return proxyFunc(r.URL)
						},
						ResponseHeaderTimeout: 8 * time.Second,
						TLSClientConfig:       tlsConfig,
						TLSHandshakeTimeout:   16 * time.Second,
					},
				}, nil
			}()
		})

		return httpClient, httpClientErr
	}

	cmd := &cobra.Command{
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
				db, err := storage.Open("/tmp/robotstxt.sqlite")
				if err != nil {
					return fmt.Errorf("open db: %v", err)
				}

				httpClient, err := httpClientFactory()
				if err != nil {
					return fmt.Errorf("http client: %v", err)
				}

				srv := server.NewService(log, db, httpClient, 30*time.Second, bot)
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

				api.RegisterServiceServer(grpcServer, srv)
			}

			return grpcServer.Serve(lis)
		},
	}

	cmd.Flags().StringVar(&bindAddress, "bind", bindAddress, "")
	cmd.Flags().StringVar(&bot.Name, "bot-name", bot.Name, "")
	cmd.Flags().StringVar(&bot.Version, "bot-version", bot.Version, "")
	cmd.Flags().StringVar(&bot.Url, "bot-url", bot.Url, "")
	cmd.Flags().StringVar(&bot.From, "bot-from", bot.From, "")
	cmd.Flags().StringVar(&httpProxy, "http-proxy", httpProxy, "")
	cmd.Flags().StringVar(&httpsProxy, "https-proxy", httpsProxy, "")
	cmd.Flags().StringVar(&caCert, "ca-cert", caCert, "")

	err := cmd.Execute()
	if err != nil {
		panic(err)
	}
}
