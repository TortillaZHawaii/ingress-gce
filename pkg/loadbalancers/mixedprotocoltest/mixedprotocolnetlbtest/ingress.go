package mixedprotocolnetlbtest

import (
	api_v1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

// IPv4Ingress is an Ingress for already existing IPv4 load balancers
func IPv4Ingress() []api_v1.LoadBalancerIngress {
	return []api_v1.LoadBalancerIngress{
		{
			IP:     IPv4Address,
			IPMode: ptr.To(api_v1.LoadBalancerIPModeVIP),
		},
	}
}

// IPv6Ingress is an Ingress for already existing IPv6 load balancers
func IPv6Ingress() []api_v1.LoadBalancerIngress {
	return []api_v1.LoadBalancerIngress{
		{
			IP:     IPv6Address,
			IPMode: ptr.To(api_v1.LoadBalancerIPModeVIP),
		},
	}
}

// DualStackIngress is an Ingress for already existing Dual Stack (IPv4 and IPv6) load balancers
func DualStackIngress() []api_v1.LoadBalancerIngress {
	return append(IPv4Ingress(), IPv6Ingress()...)
}
