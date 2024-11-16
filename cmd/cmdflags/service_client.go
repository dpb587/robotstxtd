package cmdflags

import (
	"fmt"

	"github.com/dpb587/robotstxtd/api"
	"github.com/spf13/pflag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ServiceClient struct {
	ServerAddress string
}

type ServiceClientValue struct {
	ServiceClient    api.ServiceClient
	ConnectionCloser func() error
}

func (f *ServiceClient) BindFlags(flags *pflag.FlagSet, prefix string) {
	flags.StringVar(&f.ServerAddress, prefix+"server", f.ServerAddress, "")
}

func (f *ServiceClient) New() (*ServiceClientValue, error) {
	cc, err := grpc.NewClient(
		f.ServerAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("new client: %v", err)
	}

	return &ServiceClientValue{
		ServiceClient:    api.NewServiceClient(cc),
		ConnectionCloser: cc.Close,
	}, nil
}
