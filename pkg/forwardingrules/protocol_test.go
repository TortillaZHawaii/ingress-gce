package forwardingrules_test

import (
	"testing"

	api_v1 "k8s.io/api/core/v1"
	"k8s.io/ingress-gce/pkg/composite"
	"k8s.io/ingress-gce/pkg/forwardingrules"
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

	l3FwdRule := &composite.ForwardingRule{
		IPProtocol: forwardingrules.L3Protocol,
	}
	udpFwdRule := &composite.ForwardingRule{
		IPProtocol: forwardingrules.UDPProtocol,
	}
	tcpFwdRule := &composite.ForwardingRule{
		IPProtocol: forwardingrules.TCPProtocol,
	}

	testCases := []struct {
		desc    string
		fwdRule *composite.ForwardingRule
		ports   []api_v1.ServicePort
		want    string
	}{
		{
			desc: "No ports, no rule",
			want: forwardingrules.L3Protocol,
		},
		{
			desc:    "No ports, TCP rule",
			fwdRule: tcpFwdRule,
			want:    forwardingrules.TCPProtocol,
		},
		{
			desc:    "No ports, UDP rule",
			fwdRule: udpFwdRule,
			want:    forwardingrules.UDPProtocol,
		},
		{
			desc:    "No ports, L3 rule",
			fwdRule: l3FwdRule,
			want:    forwardingrules.L3Protocol,
		},
		{
			desc:  "UDP only port, no rule",
			ports: []api_v1.ServicePort{udpPort},
			want:  forwardingrules.L3Protocol,
		},
		{
			desc:    "UDP only port, TCP rule",
			ports:   []api_v1.ServicePort{udpPort},
			fwdRule: tcpFwdRule,
			want:    forwardingrules.L3Protocol,
		},
		{
			desc:    "UDP only port, UDP rule",
			ports:   []api_v1.ServicePort{udpPort},
			fwdRule: udpFwdRule,
			want:    forwardingrules.UDPProtocol,
		},
		{
			desc:    "UDP only port, L3 rule",
			ports:   []api_v1.ServicePort{udpPort},
			fwdRule: l3FwdRule,
			want:    forwardingrules.L3Protocol,
		},
		{
			desc:  "TCP only port, no rule",
			ports: []api_v1.ServicePort{tcpPort},
			want:  forwardingrules.L3Protocol,
		},
		{
			desc:    "TCP only port, TCP rule",
			ports:   []api_v1.ServicePort{tcpPort},
			fwdRule: tcpFwdRule,
			want:    forwardingrules.TCPProtocol,
		},
		{
			desc:    "TCP only port, UDP rule",
			ports:   []api_v1.ServicePort{tcpPort},
			fwdRule: udpFwdRule,
			want:    forwardingrules.L3Protocol,
		},
		{
			desc:    "TCP only port, L3 rule",
			ports:   []api_v1.ServicePort{tcpPort},
			fwdRule: l3FwdRule,
			want:    forwardingrules.L3Protocol,
		},
		{
			desc:  "Mixed ports, TCP first, no rule",
			ports: []api_v1.ServicePort{tcpPort, udpPort},
			want:  forwardingrules.L3Protocol,
		},
		{
			desc:    "Mixed ports, TCP first, TCP rule",
			ports:   []api_v1.ServicePort{tcpPort, udpPort},
			fwdRule: tcpFwdRule,
			want:    forwardingrules.L3Protocol,
		},
		{
			desc:    "Mixed ports, TCP first, UDP rule",
			ports:   []api_v1.ServicePort{tcpPort, udpPort},
			fwdRule: udpFwdRule,
			want:    forwardingrules.L3Protocol,
		},
		{
			desc:    "Mixed ports, TCP first, L3 rule",
			ports:   []api_v1.ServicePort{tcpPort, udpPort},
			fwdRule: l3FwdRule,
			want:    forwardingrules.L3Protocol,
		},
		{
			desc:  "Mixed ports, UDP first, no rule",
			ports: []api_v1.ServicePort{udpPort, tcpPort},
			want:  forwardingrules.L3Protocol,
		},
		{
			desc:    "Mixed ports, UDP first, TCP rule",
			ports:   []api_v1.ServicePort{udpPort, tcpPort},
			fwdRule: tcpFwdRule,
			want:    forwardingrules.L3Protocol,
		},
		{
			desc:    "Mixed ports, UDP first, UDP rule",
			ports:   []api_v1.ServicePort{udpPort, tcpPort},
			fwdRule: udpFwdRule,
			want:    forwardingrules.L3Protocol,
		},
		{
			desc:    "Mixed ports, UDP first, L3 rule",
			ports:   []api_v1.ServicePort{udpPort, tcpPort},
			fwdRule: l3FwdRule,
			want:    forwardingrules.L3Protocol,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			got := forwardingrules.GetProtocol(tC.ports, tC.fwdRule)

			if got != tC.want {
				t.Errorf("GetProtocol(_, _) = %v, want %v", got, tC.want)
			}
		})
	}
}
