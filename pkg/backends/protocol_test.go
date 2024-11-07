package backends_test

import (
	"testing"

	api_v1 "k8s.io/api/core/v1"
	"k8s.io/ingress-gce/pkg/backends"
)

func TestGetProtocol(t *testing.T) {
	tcpPort := api_v1.ServicePort{
		Name:     "TCP Port",
		Protocol: api_v1.ProtocolTCP,
	}
	udpPort := api_v1.ServicePort{
		Name:     "UDP Port",
		Protocol: api_v1.ProtocolUDP,
	}

	testCases := []struct {
		ports            []api_v1.ServicePort
		expectedProtocol string
		desc             string
	}{
		{
			ports:            []api_v1.ServicePort{},
			expectedProtocol: string(api_v1.ProtocolTCP),
			desc:             "Empty ports should resolve to TCP",
		},
		{
			ports: []api_v1.ServicePort{
				udpPort,
			},
			expectedProtocol: string(api_v1.ProtocolUDP),
			desc:             "UDP protocol only",
		},
		{
			ports: []api_v1.ServicePort{
				tcpPort,
			},
			expectedProtocol: string(api_v1.ProtocolTCP),
			desc:             "TCP protocol only",
		},
		{
			ports: []api_v1.ServicePort{
				udpPort,
				tcpPort,
			},
			expectedProtocol: "UNSPECIFIED",
			desc:             "Mixed protocols, first UDP",
		},
		{
			ports: []api_v1.ServicePort{
				tcpPort,
				udpPort,
			},
			expectedProtocol: "UNSPECIFIED",
			desc:             "Mixed protocols, first TCP",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			protocol := backends.GetProtocol(tc.ports)

			if protocol != tc.expectedProtocol {
				t.Errorf("GetProtocol returned %v, not equal to expected protocol = %v", protocol, tc.expectedProtocol)
			}
		})
	}
}
