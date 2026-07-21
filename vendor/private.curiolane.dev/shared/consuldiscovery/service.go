package consuldiscovery

import (
	"fmt"

	consulapi "github.com/hashicorp/consul/api"
)

type Service struct {
	client *consulapi.Client
}

func NewService(cfg Config) (*Service, error) {
	if cfg.Address == "" {
		return nil, fmt.Errorf("address is required")
	}

	consulCfg := consulapi.DefaultConfig()
	consulCfg.Address = cfg.Address

	if cfg.Scheme != "" {
		consulCfg.Scheme = cfg.Scheme
	}

	if cfg.Datacenter != "" {
		consulCfg.Datacenter = cfg.Datacenter
	}

	if cfg.Token != "" {
		consulCfg.Token = cfg.Token
	}

	client, err := consulapi.NewClient(consulCfg)
	if err != nil {
		return nil, fmt.Errorf("new client: %w", err)
	}

	return &Service{client: client}, nil
}
