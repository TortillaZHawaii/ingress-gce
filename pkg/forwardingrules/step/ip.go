package step

import (
	api_v1 "k8s.io/api/core/v1"
	"k8s.io/ingress-gce/pkg/composite"
)

func GetIP(forwardingRules []*composite.ForwardingRule, service *api_v1.Service) (string, error) {
	return "TODO", nil
}

func HoldIP() error {
	// TODO
	return nil
}

func ReleaseIP() error {
	// TODO
	return nil
}

func AddIP(forwardingRules []*composite.ForwardingRule, ip string) []*composite.ForwardingRule {
	for _, fr := range forwardingRules {
		fr.IPAddress = ip
	}
	return forwardingRules
}
