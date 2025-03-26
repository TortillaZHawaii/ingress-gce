package forwardingrules

import (
	"fmt"

	api_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/ingress-gce/pkg/composite"
)

type CleanPortsStrategy struct {
}

type protocolPorts map[api_v1.Protocol]sets.Set[string]

func (cps *CleanPortsStrategy) Clean(forwardingRules []*composite.ForwardingRule, service *api_v1.Service) []*composite.ForwardingRule {
	cleaned := make([]*composite.ForwardingRule, 0, len(forwardingRules))
	wantedPorts := portsMap(service.Spec.Ports)

	// shallow or deep copy?
	for _, fr := range forwardingRules {
		leftPorts := make([]string, 0, len(fr.Ports))
		for _, port := range fr.Ports {
			if set, ok := wantedPorts[api_v1.Protocol(fr.IPProtocol)]; ok && set.Has(port) {
				leftPorts = append(leftPorts, port)
			}
		}
		fr.Ports = leftPorts
	}

	return cleaned
}

func portsMap(wantedPorts []api_v1.ServicePort) protocolPorts {
	m := make(protocolPorts)
	for _, port := range wantedPorts {
		if _, ok := m[port.Protocol]; !ok {
			m[port.Protocol] = sets.New[string]()
		}

		// we convert to string since that's what GCE FRs use
		portStr := fmt.Sprintf("%d", port.Port)
		m[port.Protocol].Insert(portStr)
	}
	return m
}
