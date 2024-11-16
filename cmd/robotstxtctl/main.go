package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/dpb587/robotstxtd/api"
	"github.com/dpb587/robotstxtd/cmd/cmdflags"
	"github.com/dpb587/robotstxtd/internal"
	"github.com/spf13/cobra"
)

func main() {
	var fServiceClient cmdflags.ServiceClient

	cmd := &cobra.Command{
		Use: "robotstxtctl",
	}

	{
		getBotCommand := &cobra.Command{
			Use:  "get-bot",
			Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				serviceClientValue, err := fServiceClient.New()
				if err != nil {
					return err
				}

				defer serviceClientValue.ConnectionCloser()

				client := serviceClientValue.ServiceClient

				res, err := client.GetBot(cmd.Context(), &api.GetBot_Request{})
				if err != nil {
					return fmt.Errorf("api: get: %v", err)
				}

				fmt.Fprintf(os.Stdout, "%s\n", res.Bot.ClientString())

				return nil
			},
		}

		cmd.AddCommand(getBotCommand)
	}

	{
		evalUrlCommand := &cobra.Command{
			Use:  "eval-url",
			Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				serviceClientValue, err := fServiceClient.New()
				if err != nil {
					return err
				}

				defer serviceClientValue.ConnectionCloser()

				client := serviceClientValue.ServiceClient

				res, err := client.EvalUrl(cmd.Context(), &api.EvalUrl_Request{
					Url: args[0],
				})
				if err != nil {
					return fmt.Errorf("api: get: %v", err)
				}

				switch result := res.Result.(type) {
				case *api.EvalUrl_Response_ResultError_:
					fmt.Fprintf(os.Stderr, "ERROR: %s\n", result.ResultError.ErrorMessage)

					os.Exit(1)
				case *api.EvalUrl_Response_ResultDisallow_:
					if result.ResultDisallow.MatchLineContents != nil {
						fmt.Fprintf(os.Stderr, "%s\n", *result.ResultDisallow.MatchLineContents)
					}

					os.Exit(1)
				case *api.EvalUrl_Response_ResultAllow_:
					// nop
				default:
					return fmt.Errorf("unexpected result: %#+v", result)
				}

				return nil
			},
		}

		cmd.AddCommand(evalUrlCommand)
	}

	{
		flushOriginCommand := &cobra.Command{
			Use:  "flush-origin",
			Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				serviceClientValue, err := fServiceClient.New()
				if err != nil {
					return err
				}

				defer serviceClientValue.ConnectionCloser()

				client := serviceClientValue.ServiceClient

				if !internal.ValidateCanonicalOrigin(args[0]) {
					return fmt.Errorf("validate: invalid origin: %s", args[0])
				}

				_, err = client.FlushOrigin(cmd.Context(), &api.FlushOrigin_Request{
					Origin: args[0],
				})
				if err != nil {
					return fmt.Errorf("api: flush: %v", err)
				}

				return nil
			},
		}

		cmd.AddCommand(flushOriginCommand)
	}

	{
		getOriginCommand := &cobra.Command{
			Use:  "get-origin",
			Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				serviceClientValue, err := fServiceClient.New()
				if err != nil {
					return err
				}

				defer serviceClientValue.ConnectionCloser()

				client := serviceClientValue.ServiceClient

				if !internal.ValidateCanonicalOrigin(args[0]) {
					return fmt.Errorf("validate: invalid origin: %s", args[0])
				}

				res, err := client.GetOrigin(cmd.Context(), &api.GetOrigin_Request{
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

		cmd.AddCommand(getOriginCommand)
	}

	fServiceClient.BindFlags(cmd.PersistentFlags(), "")

	err := cmd.Execute()
	if err != nil {
		panic(err)
	}
}
