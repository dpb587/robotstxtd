package cmdflags

import (
	"github.com/dpb587/robotstxtd/api"
	"github.com/spf13/pflag"
)

type Bot struct {
	Name    string
	Version string
	URL     string
	Email   string
}

func (f *Bot) BindFlags(flags *pflag.FlagSet, prefix string) {
	flags.StringVar(&f.Name, prefix+"name", f.Name, "")
	flags.StringVar(&f.Version, prefix+"version", f.Version, "")
	flags.StringVar(&f.URL, prefix+"url", f.URL, "")
	flags.StringVar(&f.Email, prefix+"email", f.Email, "")
}

func (f *Bot) NewProto() *api.Bot {
	return &api.Bot{
		Name:    f.Name,
		Version: f.Version,
		Url:     f.URL,
		Email:   f.Email,
	}
}
