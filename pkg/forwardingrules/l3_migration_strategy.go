package forwardingrules

import (
	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	api_v1 "k8s.io/api/core/v1"
	"k8s.io/ingress-gce/pkg/composite"
)

type L3MigrationStrategy struct {
	BackendServiceLink string
	Service            *api_v1.Service
	ForwardingRules    []*composite.ForwardingRule
	Provider           Provider
	Namer              Namer
}

func (l3ms *L3MigrationStrategy) Matches() bool {
	count := l3ms.portsCountWantedPerProtocol(l3ms.Service)

	for _, fr := range l3ms.ForwardingRules {
		if fr.PortRange == "" {
			continue
		}

		if count[api_v1.Protocol(fr.IPProtocol)] > MaxDiscretePorts {
			return true
		}
	}

	return false
}

func (l3ms *L3MigrationStrategy) Apply() (func() error, error) {
	name := l3ms.Namer.L4ForwardingRule(l3ms.Service.Namespace, l3ms.Service.Name, "l3tmp")
	if err := l3ms.Provider.Create(&composite.ForwardingRule{
		Name:           name,
		IPProtocol:     "L3_DEFAULT",
		AllPorts:       true,
		BackendService: l3ms.BackendServiceLink,
		Version:        meta.VersionGA,
		Scope:          meta.Regional,
	}); err != nil {
		return nil, err
	}

	return func() error {
		return l3ms.Provider.Delete(name)
	}, nil
}

func (l3ms *L3MigrationStrategy) portsCountWantedPerProtocol(service *api_v1.Service) map[api_v1.Protocol]int {
	const defaultPort = api_v1.ProtocolTCP
	count := make(map[api_v1.Protocol]int)

	for _, port := range service.Spec.Ports {
		protocol := port.Protocol
		if protocol == "" {
			protocol = defaultPort
		}

		count[protocol]++
	}

	return count
}
