package step

import "k8s.io/ingress-gce/pkg/composite"

func RemovePortRanges(forwardingRules []*composite.ForwardingRule) []*composite.ForwardingRule {
	for _, fr := range forwardingRules {
		fr.PortRange = ""
	}

	return forwardingRules
}
