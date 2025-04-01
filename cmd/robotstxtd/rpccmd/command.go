package rpccmd

import (
	"fmt"
	"os"

	"github.com/dpb587/robotstxtd/cmd/robotstxtdpflag"
	"github.com/dpb587/robotstxtd/internal"
	"github.com/dpb587/robotstxtd/robotstxtdpb"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

func NewCommand() *cobra.Command {
	var fServiceClient robotstxtdpflag.CrawlerClient
	var fOutput string

	doOutput := func(cmd *cobra.Command, msg proto.Message, df func() error) error {
		switch fOutput {
		case "", "default":
			return df()
		case "protojson":
			buf, err := protojson.Marshal(msg)
			if err != nil {
				return fmt.Errorf("marshal: %v", err)
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", buf)

			return err
		case "protobuf":
			buf, err := proto.Marshal(msg)
			if err != nil {
				return fmt.Errorf("marshal: %v", err)
			}

			_, err = cmd.OutOrStdout().Write(buf)

			return err
		}

		return fmt.Errorf("invalid output format: %s", fOutput)
	}

	cmd := &cobra.Command{
		Use: "rpc",
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

				res, err := client.GetBot(cmd.Context(), &robotstxtdpb.Crawler_GetBot_Request{})
				if err != nil {
					return fmt.Errorf("api: get: %v", err)
				}

				return doOutput(cmd, res, func() error {
					_, err := cmd.OutOrStdout().Write([]byte(prototext.Format(res)))

					return err
				})
			},
		}

		cmd.AddCommand(getBotCommand)
	}

	{
		getFileCommand := &cobra.Command{
			Use:  "get-file",
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

				res, err := client.GetFile(cmd.Context(), &robotstxtdpb.Crawler_GetFile_Request{
					ServiceUri: args[0],
				})
				if err != nil {
					return fmt.Errorf("api: get: %v", err)
				}

				return doOutput(cmd, res, func() error {
					switch upstreamResult := res.Upstream.Result.(type) {
					case *robotstxtdpb.File_Upstream_Error:
						fmt.Fprintf(os.Stderr, "ERROR: %s\n", upstreamResult.Error)

						os.Exit(1)
					case *robotstxtdpb.File_Upstream_Response_:
						if res.Body != nil {
							fmt.Fprintf(os.Stdout, "%s\n", res.Body.Text)
						}
					}

					return nil
				})
			},
		}

		cmd.AddCommand(getFileCommand)
	}

	{
		invalidateFileCacheCommand := &cobra.Command{
			Use:  "invalidate-file-cache",
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

				res, err := client.InvalidateFileCache(cmd.Context(), &robotstxtdpb.Crawler_InvalidateFileCache_Request{
					ServiceUri: args[0],
				})
				if err != nil {
					return fmt.Errorf("api: flush: %v", err)
				}

				return doOutput(cmd, res, func() error {
					return nil
				})
			},
		}

		cmd.AddCommand(invalidateFileCacheCommand)
	}

	{
		evaluateUriCommand := &cobra.Command{
			Use:  "evaluate-uri",
			Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				serviceClientValue, err := fServiceClient.New()
				if err != nil {
					return err
				}

				defer serviceClientValue.ConnectionCloser()

				client := serviceClientValue.ServiceClient

				res, err := client.EvaluateUri(cmd.Context(), &robotstxtdpb.Crawler_EvaluateUri_Request{
					Uri: args[0],
				})
				if err != nil {
					return fmt.Errorf("api: get: %v", err)
				}

				return doOutput(cmd, res, func() error {
					resStatus, err := res.ToStatusProto()
					if err != nil {
						return fmt.Errorf("api: get: %v", err)
					} else if resStatus.Code() != codes.OK {
						fmt.Fprintf(os.Stderr, "ERROR: %s\n", resStatus.Message())

						os.Exit(1)
					}

					return nil
				})
			},
		}

		cmd.AddCommand(evaluateUriCommand)
	}

	fServiceClient.BindFlags(cmd.PersistentFlags(), "")
	cmd.PersistentFlags().StringVarP(&fOutput, "output", "o", "", "output format (default, protojson, protobuf)")

	return cmd
}
