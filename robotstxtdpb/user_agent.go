package robotstxtdpb

import "fmt"

func (a *UserAgent) ClientString() string {
	return fmt.Sprintf("%s/%s (+%s)", a.Identifier, a.Version, a.Url)
}

func (a *UserAgent) FeatureString() string {
	return fmt.Sprintf("%s/%s; +%s", a.Identifier, a.Version, a.Url)
}
