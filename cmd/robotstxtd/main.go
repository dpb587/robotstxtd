package main

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/dpb587/robotstxtd/agent"
	"github.com/dpb587/robotstxtd/api"
	"github.com/dpb587/robotstxtd/server"
	"github.com/dpb587/robotstxtd/server/storage"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

func main() {
	log := slog.Default()

	var bindAddress string = "127.0.0.1:0"
	var bot agent.Bot

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

				srv := server.NewService(log, db, http.DefaultClient, 30*time.Second, bot)
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
	cmd.Flags().StringVar(&bot.URL, "bot-url", bot.URL, "")

	err := cmd.Execute()
	if err != nil {
		panic(err)
	}
}
