package forwardingrules

import (
	api_v1 "k8s.io/api/core/v1"
	"k8s.io/ingress-gce/pkg/composite"
)

type L3MigrationStrategy struct {
}

func (l3ms *L3MigrationStrategy) Matches(service *api_v1.Service, forwardingRules []*composite.ForwardingRule) bool {
	if l3ms.containsRangeForwardingRule(forwardingRules) && l3ms.requiresMultipleDiscreteForwardingRulesPerProtocol(service) {
		return true
	}
	return false
}

func (l3ms *L3MigrationStrategy) Apply(service *api_v1.Service, provider Provider, namer Namer) (func() error, error) {
	name := namer.L4ForwardingRule(service.Namespace, service.Name, "l3tmp")
	if err := provider.Create(&composite.ForwardingRule{}); err != nil {
		return nil, err
	}

	return func() error {
		return provider.Delete(name)
	}, nil
}

func (l3ms *L3MigrationStrategy) containsRangeForwardingRule(frs []*composite.ForwardingRule) bool {
	for _, fr := range frs {
		if len(fr.Ports) == 0 && fr.PortRange != "" {
			return true
		}
	}
	return false
}

func (l3ms *L3MigrationStrategy) requiresMultipleDiscreteForwardingRulesPerProtocol(service *api_v1.Service) bool {
	const defaultPort = api_v1.ProtocolTCP
	portsCountPerProtocol := make(map[api_v1.Protocol]int)

	for _, port := range service.Spec.Ports {
		protocol := port.Protocol
		if protocol == "" {
			protocol = defaultPort
		}

		portsCountPerProtocol[protocol]++
	}

	for _, count := range portsCountPerProtocol {
		if count > MaxDiscretePorts {
			return true
		}
	}
	return false
}
