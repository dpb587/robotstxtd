package main

import (
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/dpb587/robotstxtd/api"
	"github.com/dpb587/robotstxtd/internal"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// log := slog.Default()

	var serverAddress string

	var (
		clientConn     *grpc.ClientConn
		clientConnErr  error
		clientConnOnce sync.Once
	)

	defer func() {
		if clientConn == nil {
			return
		}

		clientConn.Close()
	}()

	clientConnFactory := func() (*grpc.ClientConn, error) {
		clientConnOnce.Do(func() {
			clientConn, clientConnErr = grpc.NewClient(
				serverAddress,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			)
		})

		return clientConn, clientConnErr
	}

	cmd := &cobra.Command{
		Use: "robotstxtctl",
	}

	{
		evalCommand := &cobra.Command{
			Use:  "eval",
			Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cc, err := clientConnFactory()
				if err != nil {
					return fmt.Errorf("client: %v", err)
				}

				client := api.NewServiceClient(cc)

				res, err := client.Eval(cmd.Context(), &api.Eval_Request{
					Url: args[0],
				})
				if err != nil {
					return fmt.Errorf("api: get: %v", err)
				}

				switch result := res.Result.(type) {
				case *api.Eval_Response_ResultError_:
					fmt.Fprintf(os.Stderr, "ERROR: %s\n", result.ResultError.ErrorMessage)

					os.Exit(1)
				case *api.Eval_Response_ResultDisallow_:
					if result.ResultDisallow.MatchLineContents != nil {
						fmt.Fprintf(os.Stderr, "%s\n", *result.ResultDisallow.MatchLineContents)
					}

					os.Exit(1)
				case *api.Eval_Response_ResultAllow_:
					// nop
				default:
					return fmt.Errorf("unexpected result: %#+v", result)
				}

				return nil
			},
		}

		cmd.AddCommand(evalCommand)
	}

	{
		flushCommand := &cobra.Command{
			Use:  "flush",
			Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cc, err := clientConnFactory()
				if err != nil {
					return fmt.Errorf("client: %v", err)
				}

				client := api.NewServiceClient(cc)

				if !internal.ValidateCanonicalOrigin(args[0]) {
					return fmt.Errorf("validate: invalid origin: %s", args[0])
				}

				_, err = client.Flush(cmd.Context(), &api.Flush_Request{
					Origin: args[0],
				})
				if err != nil {
					return fmt.Errorf("api: flush: %v", err)
				}

				return nil
			},
		}

		cmd.AddCommand(flushCommand)
	}

	{
		getCommand := &cobra.Command{
			Use:  "get",
			Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cc, err := clientConnFactory()
				if err != nil {
					return fmt.Errorf("client: %v", err)
				}

				client := api.NewServiceClient(cc)

				if !internal.ValidateCanonicalOrigin(args[0]) {
					return fmt.Errorf("validate: invalid origin: %s", args[0])
				}

				res, err := client.Get(cmd.Context(), &api.Get_Request{
					Origin: args[0],
				})
				if err != nil {
					return fmt.Errorf("api: get: %v", err)
				}

				if res.ResponseHeaders != nil {
					fmt.Fprintf(os.Stdout, "HTTP: %d %s\n", res.ResponseStatusCode, http.StatusText(int(res.ResponseStatusCode)))
					fmt.Fprintf(os.Stdout, "%s\n", *res.ResponseHeaders)
				}

				if res.ResponseBody != nil {
					fmt.Fprintf(os.Stdout, "\n%s\n", *res.ResponseBody)
				}

				if res.ErrorMessage != nil {
					fmt.Fprintf(os.Stderr, "ERROR: %s\n", *res.ErrorMessage)

					os.Exit(1)
				}

				return nil
			},
		}

		cmd.AddCommand(getCommand)
	}

	cmd.PersistentFlags().StringVar(&serverAddress, "server", serverAddress, "")

	err := cmd.Execute()
	if err != nil {
		panic(err)
	}
}
