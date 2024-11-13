package agent

import "fmt"

type Bot struct {
	Name    string
	Version string
	URL     string
}

func (a Bot) ClientString() string {
	return fmt.Sprintf("%s/%s (+%s)", a.Name, a.Version, a.URL)
}

func (a Bot) FeatureString() string {
	return fmt.Sprintf("%s/%s; +%s", a.Name, a.Version, a.URL)
}
