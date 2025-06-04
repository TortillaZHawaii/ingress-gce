package steps_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/ingress-gce/pkg/composite"
	"k8s.io/ingress-gce/pkg/forwardingrules/optimized/steps"
)

func TestRemoveEmptyForwardingRules(t *testing.T) {
	testCases := []struct {
		desc string
		have []*composite.ForwardingRule
		want []*composite.ForwardingRule
	}{
		{
			desc: "empty",
			have: []*composite.ForwardingRule{},
			want: []*composite.ForwardingRule{},
		},
		{
			desc: "one empty rule",
			have: []*composite.ForwardingRule{
				{Ports: []string{}},
				{Ports: []string{"80"}},
			},
			want: []*composite.ForwardingRule{
				{Ports: []string{"80"}},
			},
		},
		{
			desc: "skip port ranges and allPorts", // this should not be present at this stage, however if it is we do nothing
			have: []*composite.ForwardingRule{
				{PortRange: "80-81"},
				{AllPorts: true},
			},
			want: []*composite.ForwardingRule{
				{PortRange: "80-81"},
				{AllPorts: true},
			},
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			// Act
			got, err := steps.RemoveEmptyForwardingRules(nil, tC.have)

			// Assert
			if err != nil {
				t.Fatalf("RemoveEmptyForwardingRules() returned error: %v, this should never fail", err)
			}

			if diff := cmp.Diff(tC.want, got); diff != "" {
				t.Errorf("want != got, (-want, +got):\n%s", diff)
			}
		})
	}
}
