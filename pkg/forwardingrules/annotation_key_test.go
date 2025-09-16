package forwardingrules_test

import (
	"testing"

	"k8s.io/ingress-gce/pkg/composite"
	"k8s.io/ingress-gce/pkg/forwardingrules"
)

func TestAnnotationKey(t *testing.T) {
	testCases := []struct {
		desc string
		have *composite.ForwardingRule
		want string
	}{
		{
			desc: "TCP IPv4",
			have: &composite.ForwardingRule{
				IpVersion:  "IPV4",
				IPProtocol: "TCP",
			},
			want: "service.kubernetes.io/tcp-forwarding-rule",
		},
		{
			desc: "UDP IPv6",
			have: &composite.ForwardingRule{
				IpVersion:  "IPV6",
				IPProtocol: "UDP",
			},
			want: "service.kubernetes.io/udp-forwarding-rule-ipv6",
		},
		{
			desc: "L3 IPv4",
			have: &composite.ForwardingRule{
				IpVersion:  "",
				IPProtocol: "L3_DEFAULT",
			},
			want: "service.kubernetes.io/l3-forwarding-rule",
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			got := forwardingrules.AnnotationKey(tC.have)
			if got != tC.want {
				t.Errorf("AnnotationKey(%v) = %v, want %v", tC.have, got, tC.want)
			}
		})
	}
}
