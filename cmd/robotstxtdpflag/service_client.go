package robotstxtdpflag

import (
	"context"
	"fmt"
	"net"

	"github.com/dpb587/robotstxtd/robotstxtdpb"
	"github.com/spf13/pflag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type CrawlerClient struct {
	ServerAddress string `yaml:"address"`

	// Dialer optionally overrides how the connection to ServerAddress is
	// established (e.g. to resolve the address through a service discovery
	// mechanism other than DNS). When set, grpc's own name resolution is
	// bypassed since ServerAddress is only used as an opaque target label.
	Dialer func(ctx context.Context, network, addr string) (net.Conn, error) `yaml:"-"`
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

	target := f.ServerAddress

	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	if f.Dialer != nil {
		target = "passthrough:///" + target
		dialOpts = append(dialOpts, grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return f.Dialer(ctx, "tcp", addr)
		}))
	}

	cc, err := grpc.NewClient(target, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("new client: %v", err)
	}

	return &ServiceClientValue{
		ServiceClient:    robotstxtdpb.NewCrawlerClient(cc),
		ConnectionCloser: cc.Close,
	}, nil
}
