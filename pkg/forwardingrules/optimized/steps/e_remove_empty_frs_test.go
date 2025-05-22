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
		have map[steps.ResourceName]*composite.ForwardingRule
		want map[steps.ResourceName]*composite.ForwardingRule
	}{
		{
			desc: "empty",
			have: map[steps.ResourceName]*composite.ForwardingRule{},
			want: map[steps.ResourceName]*composite.ForwardingRule{},
		},
		{
			desc: "one empty rule",
			have: map[steps.ResourceName]*composite.ForwardingRule{
				"rule-1": {Ports: []string{}},
				"rule-2": {Ports: []string{"80"}},
			},
			want: map[steps.ResourceName]*composite.ForwardingRule{
				"rule-2": {Ports: []string{"80"}},
			},
		},
		{
			desc: "skip port ranges and allPorts", // this should not be present at this stage, however if it is we do nothing
			have: map[steps.ResourceName]*composite.ForwardingRule{
				"rule-1": {PortRange: "80-81"},
				"rule-2": {AllPorts: true},
			},
			want: map[steps.ResourceName]*composite.ForwardingRule{
				"rule-1": {PortRange: "80-81"},
				"rule-2": {AllPorts: true},
			},
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			// Act
			if err := steps.RemoveEmptyForwardingRules(nil, tC.have); err != nil {
				t.Fatalf("RemoveEmptyForwardingRules() returned error: %v, this should never fail", err)
			}

			// Assert
			if diff := cmp.Diff(tC.want, tC.have); diff != "" {
				t.Errorf("want != got, (-want, +got):\n%s", diff)
			}
		})
	}
}
