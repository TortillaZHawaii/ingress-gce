package backends

import api_v1 "k8s.io/api/core/v1"

// For mixed service ports we want to create UNSPECIFIED backend service.
// If there are only protocols of one type return that type.
//
// See https://cloud.google.com/load-balancing/docs/internal#forwarding-rule-protocols
func GetProtocol(svcPorts []api_v1.ServicePort) string {
	protocolSet := make(map[api_v1.Protocol]struct{})
	for _, port := range svcPorts {
		protocolSet[port.Protocol] = struct{}{}
	}

	_, okTCP := protocolSet[api_v1.ProtocolTCP]
	_, okUDP := protocolSet[api_v1.ProtocolUDP]

	switch {
	case okTCP && okUDP:
		// L3 Backend service is created with UNSPECIFIED protocol.
		return "UNSPECIFIED"
	case okUDP:
		return string(api_v1.ProtocolUDP)
	default: // TCP or no protocols
		return string(api_v1.ProtocolTCP)
	}
}
