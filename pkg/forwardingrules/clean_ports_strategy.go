package forwardingrules

import (
	"fmt"

	api_v1 "k8s.io/api/core/v1"
	"k8s.io/ingress-gce/pkg/composite"
)

type CleanPortsStrategy struct {
}

func (cps *CleanPortsStrategy) Clean(forwardingRules []*composite.ForwardingRule, service *api_v1.Service) ([]*composite.ForwardingRule, error) {
	cleaned := make([]*composite.ForwardingRule, 0, len(forwardingRules))
	wantedPorts := portsMap(service.Spec.Ports)

	// shallow or deep copy?
	for _, fr := range forwardingRules {
		leftPorts := make([]string, 0, len(fr.Ports))
		for _, port := range fr.Ports {
			if _, ok := wantedPorts[api_v1.Protocol(fr.IPProtocol)][port]; ok {
				leftPorts = append(leftPorts, port)
			}
		}
		fr.Ports = leftPorts
	}

	return cleaned, nil
}

func portsMap(wantedPorts []api_v1.ServicePort) map[api_v1.Protocol]map[string]struct{} {
	m := make(map[api_v1.Protocol]map[string]struct{})
	for _, port := range wantedPorts {
		// we convert to string since that's what FRs use for port
		m[port.Protocol][fmt.Sprintf("%d", port.Port)] = struct{}{}
	}
	return m
}
