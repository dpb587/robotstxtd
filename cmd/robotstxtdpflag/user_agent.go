package robotstxtdpflag

import (
	"github.com/dpb587/robotstxtd/robotstxtdpb"
	"github.com/spf13/pflag"
)

type UserAgent struct {
	Identifier string
	Version    string
	URL        string
	Email      string
}

func (f *UserAgent) BindFlags(flags *pflag.FlagSet, prefix string) {
	flags.StringVar(&f.Identifier, prefix+"identifier", f.Identifier, "")
	flags.StringVar(&f.Version, prefix+"version", f.Version, "")
	flags.StringVar(&f.URL, prefix+"url", f.URL, "")
	flags.StringVar(&f.Email, prefix+"email", f.Email, "")
}

func (f *UserAgent) NewProto() *robotstxtdpb.UserAgent {
	return &robotstxtdpb.UserAgent{
		Identifier: f.Identifier,
		Version:    f.Version,
		Url:        f.URL,
		Email:      f.Email,
	}
}
