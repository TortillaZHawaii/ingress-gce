package steps

import (
	api_v1 "k8s.io/api/core/v1"
	"k8s.io/ingress-gce/pkg/composite"
)

// RemoveEmptyForwardingRules removes Forwarding Rules that don't specify any discrete ports.
// frs map is mutated.
func RemoveEmptyForwardingRules(_ []api_v1.ServicePort, frs []*composite.ForwardingRule) ([]*composite.ForwardingRule, error) {
	nonEmptyFRs := make([]*composite.ForwardingRule, 0, len(frs))
	for _, fr := range frs {
		isNonDiscrete := len(fr.PortRange) > 0 || fr.AllPorts
		hasPorts := len(fr.Ports) > 0

		if isNonDiscrete || hasPorts {
			nonEmptyFRs = append(nonEmptyFRs, fr)
		}
	}

	return nonEmptyFRs, nil
}
