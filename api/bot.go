package api

import "fmt"

func (a *Bot) ClientString() string {
	return fmt.Sprintf("%s/%s (+%s)", a.Name, a.Version, a.Url)
}

func (a *Bot) FeatureString() string {
	return fmt.Sprintf("%s/%s; +%s", a.Name, a.Version, a.Url)
}
