package consuldiscovery

import "time"

const DefaultRefreshInterval = 30 * time.Second

type Config struct {
	Address         string        `yaml:"address"`
	Scheme          string        `yaml:"scheme"`
	Datacenter      string        `yaml:"datacenter"`
	Token           string        `yaml:"token"`
	RefreshInterval time.Duration `yaml:"refreshInterval"`
}
