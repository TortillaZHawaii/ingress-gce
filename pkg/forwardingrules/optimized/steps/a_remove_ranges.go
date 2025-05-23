package steps

import "k8s.io/ingress-gce/pkg/composite"

func RemovePortRanges(frs map[ResourceName]*composite.ForwardingRule) map[ResourceName]*composite.ForwardingRule {
	for _, fr := range frs {
		fr.PortRange = ""
	}
	return frs
}
