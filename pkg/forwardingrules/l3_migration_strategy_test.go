package forwardingrules

import (
	"testing"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	api_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cloud-provider-gcp/providers/gce"
	"k8s.io/ingress-gce/pkg/composite"
	"k8s.io/ingress-gce/pkg/utils/namer"
	"k8s.io/klog/v2"
)

func TestMatches(t *testing.T) {
	testCases := []struct {
		desc            string
		ports           []api_v1.ServicePort
		forwardingRules []*composite.ForwardingRule
		want            bool
	}{
		{
			desc: "create, single port",
			ports: []api_v1.ServicePort{
				{Protocol: api_v1.ProtocolTCP, Port: 80},
			},
			forwardingRules: nil,
			want:            false,
		},
		{
			desc: "modify existing, single port",
			ports: []api_v1.ServicePort{
				{
					Protocol: api_v1.ProtocolTCP,
					Port:     80,
				},
			},
			forwardingRules: []*composite.ForwardingRule{
				{
					IPProtocol: "TCP",
					Ports:      []string{"80"},
				},
			},
			want: false,
		},
		{
			desc: "modify existing, 5 discrete ports",
			ports: []api_v1.ServicePort{
				{Protocol: api_v1.ProtocolTCP, Port: 80},
				{Protocol: api_v1.ProtocolTCP, Port: 81},
				{Protocol: api_v1.ProtocolTCP, Port: 82},
				{Protocol: api_v1.ProtocolTCP, Port: 83},
				{Protocol: api_v1.ProtocolTCP, Port: 84},
			},
			forwardingRules: []*composite.ForwardingRule{
				{
					IPProtocol: "TCP",
					Ports:      []string{"80", "81", "82", "83", "84"},
				},
			},
			want: false,
		},
		{
			desc: "modify existing, 5 ports in a range",
			ports: []api_v1.ServicePort{
				{Protocol: api_v1.ProtocolTCP, Port: 80},
				{Protocol: api_v1.ProtocolTCP, Port: 81},
				{Protocol: api_v1.ProtocolTCP, Port: 82},
				{Protocol: api_v1.ProtocolTCP, Port: 83},
				{Protocol: api_v1.ProtocolTCP, Port: 84},
			},
			forwardingRules: []*composite.ForwardingRule{
				{
					IPProtocol: "TCP",
					PortRange:  "80-84",
				},
			},
			// this will be fixed with mutability
			want: false,
		},
		{
			desc: "modify existing, 6 ports, 2 discrete rules",
			ports: []api_v1.ServicePort{
				{Protocol: api_v1.ProtocolTCP, Port: 80},
				{Protocol: api_v1.ProtocolTCP, Port: 81},
				{Protocol: api_v1.ProtocolTCP, Port: 82},
				{Protocol: api_v1.ProtocolTCP, Port: 83},
				{Protocol: api_v1.ProtocolTCP, Port: 84},
				{
					// default protocol should be TCP
					Port: 85,
				},
			},
			forwardingRules: []*composite.ForwardingRule{
				{
					IPProtocol: "TCP",
					Ports:      []string{"80", "81", "82", "83", "84"},
				},
				{
					IPProtocol: "TCP",
					Ports:      []string{"85"},
				},
			},
			want: false,
		},
		{
			desc: "modify existing, 6 ports, 1 range",
			ports: []api_v1.ServicePort{
				{Protocol: api_v1.ProtocolTCP, Port: 80},
				{Protocol: api_v1.ProtocolTCP, Port: 81},
				{Protocol: api_v1.ProtocolTCP, Port: 82},
				{Protocol: api_v1.ProtocolTCP, Port: 83},
				{Protocol: api_v1.ProtocolTCP, Port: 84},
				{
					// default protocol should be TCP
					Port: 85,
				},
			},
			forwardingRules: []*composite.ForwardingRule{
				{
					IPProtocol: "TCP",
					PortRange:  "80-85",
				},
			},
			want: true,
		},
		{
			desc: "modify existing, 3 TCP, 3 UDP ports, 2 range rules",
			ports: []api_v1.ServicePort{
				{Protocol: api_v1.ProtocolTCP, Port: 80},
				{Protocol: api_v1.ProtocolTCP, Port: 81},
				{Protocol: api_v1.ProtocolTCP, Port: 82},
				{Protocol: api_v1.ProtocolUDP, Port: 80},
				{Protocol: api_v1.ProtocolUDP, Port: 81},
				{Protocol: api_v1.ProtocolUDP, Port: 82},
			},
			forwardingRules: []*composite.ForwardingRule{
				{
					IPProtocol: "TCP",
					PortRange:  "80-82",
				},
				{
					IPProtocol: "UDP",
					PortRange:  "80-82",
				},
			},
			want: false,
		},
		{
			desc: "modify existing, 3 TCP, 3 UDP ports, 2 discrete rules",
			ports: []api_v1.ServicePort{
				{Protocol: api_v1.ProtocolTCP, Port: 80},
				{Protocol: api_v1.ProtocolTCP, Port: 81},
				{Protocol: api_v1.ProtocolTCP, Port: 82},
				{Protocol: api_v1.ProtocolUDP, Port: 80},
				{Protocol: api_v1.ProtocolUDP, Port: 81},
				{Protocol: api_v1.ProtocolUDP, Port: 82},
			},
			forwardingRules: []*composite.ForwardingRule{
				{
					IPProtocol: "TCP",
					Ports:      []string{"80", "81", "82"},
				},
				{
					IPProtocol: "UDP",
					Ports:      []string{"80", "81", "82"},
				},
			},
			want: false,
		},
		{
			desc: "modify existing, 3 TCP, 6 UDP ports, discrete TCP, range UDP rules (shouldn't be possible)",
			ports: []api_v1.ServicePort{
				{Protocol: api_v1.ProtocolTCP, Port: 80},
				{Protocol: api_v1.ProtocolTCP, Port: 81},
				{Protocol: api_v1.ProtocolTCP, Port: 82},
				{Protocol: api_v1.ProtocolUDP, Port: 80},
				{Protocol: api_v1.ProtocolUDP, Port: 81},
				{Protocol: api_v1.ProtocolUDP, Port: 82},
				{Protocol: api_v1.ProtocolUDP, Port: 83},
				{Protocol: api_v1.ProtocolUDP, Port: 84},
				{Protocol: api_v1.ProtocolUDP, Port: 85},
			},
			forwardingRules: []*composite.ForwardingRule{
				{
					IPProtocol: "TCP",
					Ports:      []string{"80", "81", "82"},
				},
				{
					IPProtocol: "UDP",
					PortRange:  "80-85",
				},
			},
			want: true,
		},
		{
			desc: "modify existing, 3 TCP, 6 UDP ports, range TCP, 2 discrete UDP rules (shouldn't be possible)",
			ports: []api_v1.ServicePort{
				{Protocol: api_v1.ProtocolTCP, Port: 80},
				{Protocol: api_v1.ProtocolTCP, Port: 81},
				{Protocol: api_v1.ProtocolTCP, Port: 82},
				{Protocol: api_v1.ProtocolUDP, Port: 80},
				{Protocol: api_v1.ProtocolUDP, Port: 81},
				{Protocol: api_v1.ProtocolUDP, Port: 82},
				{Protocol: api_v1.ProtocolUDP, Port: 83},
				{Protocol: api_v1.ProtocolUDP, Port: 84},
				{Protocol: api_v1.ProtocolUDP, Port: 85},
			},
			forwardingRules: []*composite.ForwardingRule{
				{
					IPProtocol: "TCP",
					PortRange:  "80-82",
				},
				{
					IPProtocol: "UDP",
					Ports:      []string{"80", "81", "82", "83", "84"},
				},
				{
					IPProtocol: "UDP",
					Ports:      []string{"85"},
				},
			},
			want: false,
		},
		{
			desc: "modify existing, 3 TCP, 6 UDP ports, range TCP, discrete UDP rules (shouldn't be possible)",
			ports: []api_v1.ServicePort{
				{Protocol: api_v1.ProtocolTCP, Port: 80},
				{Protocol: api_v1.ProtocolTCP, Port: 81},
				{Protocol: api_v1.ProtocolTCP, Port: 82},
				{Protocol: api_v1.ProtocolUDP, Port: 80},
				{Protocol: api_v1.ProtocolUDP, Port: 81},
				{Protocol: api_v1.ProtocolUDP, Port: 82},
				{Protocol: api_v1.ProtocolUDP, Port: 83},
				{Protocol: api_v1.ProtocolUDP, Port: 84},
				{Protocol: api_v1.ProtocolUDP, Port: 85},
			},
			forwardingRules: []*composite.ForwardingRule{
				{
					IPProtocol: "TCP",
					PortRange:  "80-82",
					Ports:      []string{"80", "81", "82"},
				},
				{
					IPProtocol: "UDP",
					Ports:      []string{"80", "81", "82", "83", "84"},
				},
				{
					IPProtocol: "UDP",
					Ports:      []string{"85"},
				},
			},
			want: false,
		},
		{
			desc: "modify existing, 3 TCP, 6 UDP ports, 3 discrete rules",
			ports: []api_v1.ServicePort{
				{Protocol: api_v1.ProtocolTCP, Port: 80},
				{Protocol: api_v1.ProtocolTCP, Port: 81},
				{Protocol: api_v1.ProtocolTCP, Port: 82},
				{Protocol: api_v1.ProtocolUDP, Port: 80},
				{Protocol: api_v1.ProtocolUDP, Port: 81},
				{Protocol: api_v1.ProtocolUDP, Port: 82},
				{Protocol: api_v1.ProtocolUDP, Port: 83},
				{Protocol: api_v1.ProtocolUDP, Port: 84},
				{Protocol: api_v1.ProtocolUDP, Port: 85},
			},
			forwardingRules: []*composite.ForwardingRule{
				{
					IPProtocol: "TCP",
					Ports:      []string{"80", "81", "82"},
				},
				{
					IPProtocol: "UDP",
					Ports:      []string{"80", "81", "82"},
				},
				{
					IPProtocol: "UDP",
					Ports:      []string{"83", "84", "85"},
				},
			},
			want: false,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			t.Parallel()

			// Arrange
			service := &api_v1.Service{
				Spec: api_v1.ServiceSpec{
					Ports: tC.ports,
				},
			}
			strategy := &L3MigrationStrategy{
				ForwardingRules: tC.forwardingRules,
				Service:         service,
			}

			// Act
			got := strategy.Matches()

			// Assert
			if got != tC.want {
				t.Errorf("Matches(_, _,) = %v, want %v", got, tC.want)
			}
		})
	}
}

func TestApply(t *testing.T) {
	testCases := []struct {
		namespace    string
		name         string
		wantL3FrName string
	}{
		{
			namespace:    "default",
			name:         "test-service",
			wantL3FrName: "k8s2-l3tmp-mtkhwubd-default-test-service-hiv2inat",
		},
		{
			namespace:    "my-namespace",
			name:         "store-frontend",
			wantL3FrName: "k8s2-l3tmp-mtkhwubd-my-namespace-store-frontend-8qr01sqx",
		},
		{
			namespace:    "default",
			name:         "surprisingly-extremely-long-name-for-the-service-exceeding-gce-length-limit",
			wantL3FrName: "k8s2-l3tmp-mtkhwubd-def-surprisingly-extremely-long-na-tc4wi59r",
		},
	}
	for _, tC := range testCases {
		desc := tC.namespace + " " + tC.name
		t.Run(desc, func(t *testing.T) {
			t.Parallel()

			// Arrange
			service := &api_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: tC.namespace,
					Name:      tC.name,
				},
			}
			vals := gce.DefaultTestClusterValues()
			fakeGCE := gce.NewFakeGCECloud(vals)
			provider := New(fakeGCE, meta.VersionGA, meta.Regional, klog.TODO())
			namer := namer.NewL4Namer("123", nil)

			strategy := &L3MigrationStrategy{
				BackendServiceLink: "test-bs",
				Service:            service,
				Provider:           provider,
				Namer:              namer,
			}

			// Act (create)
			release, err := strategy.Apply()

			// Assert (create)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if release == nil {
				t.Fatal("release is nil")
			}
			got, err := provider.Get(tC.wantL3FrName)

			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}

			ignore := cmpopts.IgnoreFields(composite.ForwardingRule{}, "Region", "SelfLink")
			want := &composite.ForwardingRule{
				Name:           tC.wantL3FrName,
				IPProtocol:     "L3_DEFAULT",
				AllPorts:       true,
				Version:        meta.VersionGA,
				Scope:          meta.Regional,
				BackendService: "test-bs",
			}
			if diff := cmp.Diff(want, got, ignore); diff != "" {
				t.Fatalf("ForwardingRule mismatch (-want +got)}: %s", diff)
			}

			// Act (release)
			err = release()

			// Assert (release)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got, err := provider.Get(tC.wantL3FrName); err != nil {
				t.Fatalf("unexpected err: %v", err)
			} else if got != nil {
				t.Fatalf("%s not deleted", tC.wantL3FrName)
			}
		})
	}
}
