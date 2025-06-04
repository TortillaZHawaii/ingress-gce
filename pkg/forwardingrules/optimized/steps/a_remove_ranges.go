package steps

import (
	api_v1 "k8s.io/api/core/v1"
	"k8s.io/ingress-gce/pkg/composite"
)

// RemovePortRanges from Forwarding Rules.
func RemovePortRanges(_ []api_v1.ServicePort, frs []*composite.ForwardingRule) ([]*composite.ForwardingRule, error) {
	for _, fr := range frs {
		fr.PortRange = ""
	}
	return frs, nil
}
