package robotstxtdpflag

import (
	"fmt"

	"github.com/dpb587/robotstxtd/robotstxtdpb"
	"github.com/spf13/pflag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type CrawlerClient struct {
	ServerAddress string `yaml:"address"`
}

type ServiceClientValue struct {
	ServiceClient    robotstxtdpb.CrawlerClient
	ConnectionCloser func() error
}

func (f *CrawlerClient) BindFlags(flags *pflag.FlagSet, prefix string) {
	flags.StringVar(&f.ServerAddress, prefix+"server", f.ServerAddress, "")
}

func (f *CrawlerClient) New() (*ServiceClientValue, error) {
	if len(f.ServerAddress) == 0 {
		return nil, fmt.Errorf("missing server address")
	}

	cc, err := grpc.NewClient(
		f.ServerAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("new client: %v", err)
	}

	return &ServiceClientValue{
		ServiceClient:    robotstxtdpb.NewCrawlerClient(cc),
		ConnectionCloser: cc.Close,
	}, nil
}
