package consuldiscovery

import "strings"

const ServiceDomainSuffix = ".service.consul"

// ParseHostname reports whether host (without any port) uses the Consul DNS
// service naming convention (e.g. "robotstxtd.service.consul"), returning the
// bare service name to look up in the catalog.
func ParseHostname(host string) (service string, ok bool) {
	if !strings.HasSuffix(strings.ToLower(host), ServiceDomainSuffix) {
		return "", false
	}

	return host[:len(host)-len(ServiceDomainSuffix)], true
}
