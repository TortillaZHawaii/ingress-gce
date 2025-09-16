package forwardingrules

import (
	"k8s.io/ingress-gce/pkg/annotations"
	"k8s.io/ingress-gce/pkg/composite"
)

func AnnotationKey(fr *composite.ForwardingRule) string {
	m := map[string]map[string]string{
		"IPV4": {
			ProtocolL3:  annotations.L3ForwardingRuleKey,
			ProtocolTCP: annotations.TCPForwardingRuleKey,
			ProtocolUDP: annotations.UDPForwardingRuleKey,
		},
		"IPV6": {
			ProtocolL3:  annotations.L3ForwardingRuleIPv6Key,
			ProtocolTCP: annotations.TCPForwardingRuleIPv6Key,
			ProtocolUDP: annotations.UDPForwardingRuleIPv6Key,
		},
	}

	version := fr.IpVersion
	if version == "" {
		version = "IPV4"
	}

	protocol := fr.IPProtocol
	if protocol == "" {
		protocol = "UDP"
	}

	return m[version][protocol]
}
