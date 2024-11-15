package forwardingrules

import (
	api_v1 "k8s.io/api/core/v1"
	"k8s.io/ingress-gce/pkg/composite"
)

const (
	L3Protocol  = "L3_DEFAULT"
	TCPProtocol = "TCP"
	UDPProtocol = "UDP"
)

// GetProtocol returns the protocol for the Forwarding Rule based on Kubernetes Service port definitions
// and existing backend service.
//
// If the service exists and service protocols match existingBS return that protocol.
// Otherwise prefer L3.
//
// See https://cloud.google.com/load-balancing/docs/internal#forwarding-rule-protocols
func GetProtocol(svcPorts []api_v1.ServicePort, existingFwdRule *composite.ForwardingRule) string {
	if doesntExist := existingFwdRule == nil; doesntExist {
		return L3Protocol
	}

	if alreadyL3 := existingFwdRule.IPProtocol == L3Protocol; alreadyL3 {
		return L3Protocol
	}

	if len(svcPorts) == 0 {
		return existingFwdRule.IPProtocol
	}

	requiredProtocol := protocolRequiredForService(svcPorts)
	needsChangeCausingTrafficInterruption := existingFwdRule.IPProtocol != requiredProtocol

	if needsChangeCausingTrafficInterruption {
		return L3Protocol
	}

	// rule exists, we don't want to create traffic interruption
	return requiredProtocol
}

func protocolRequiredForService(svcPorts []api_v1.ServicePort) string {
	protocolSet := make(map[api_v1.Protocol]struct{})
	for _, port := range svcPorts {
		protocolSet[port.Protocol] = struct{}{}
	}

	_, okTCP := protocolSet[api_v1.ProtocolTCP]
	_, okUDP := protocolSet[api_v1.ProtocolUDP]

	switch {
	case okTCP && okUDP:
		return L3Protocol
	case okUDP:
		return UDPProtocol
	case okTCP:
		return TCPProtocol
	default:
		return L3Protocol
	}
}
