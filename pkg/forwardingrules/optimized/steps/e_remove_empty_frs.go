package steps

import (
	api_v1 "k8s.io/api/core/v1"
	"k8s.io/ingress-gce/pkg/composite"
)

// RemoveEmptyForwardingRules removes Forwarding Rules that don't specify any discrete ports.
// frs map is mutated.
func RemoveEmptyForwardingRules(_ []api_v1.ServicePort, frs map[ResourceName]*composite.ForwardingRule) error {
	for name, fr := range frs {
		if isNonDiscrete := len(fr.PortRange) > 0 || fr.AllPorts; isNonDiscrete {
			continue
		}

		if isEmpty := len(fr.Ports) == 0; isEmpty {
			delete(frs, name)
		}
	}
	return nil
}
