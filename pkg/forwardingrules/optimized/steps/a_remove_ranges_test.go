package steps_test

import (
	"testing"

	"k8s.io/ingress-gce/pkg/composite"
	"k8s.io/ingress-gce/pkg/forwardingrules/optimized/steps"
)

func TestRemovePortRanges(t *testing.T) {
	testCases := []struct {
		desc string
	}{
		{
			desc: "empty",
			have: []*composite.ForwardingRule{},

			want: []*composite.ForwardingRule{},
		},
	}
	for _, tC := range testCases {
		tC := tC
		t.Run(tC.desc, func(t *testing.T) {
			t.Parallel()

			// Act
			got, err := steps.RemovePortRanges(nil)
		})
	}
}
