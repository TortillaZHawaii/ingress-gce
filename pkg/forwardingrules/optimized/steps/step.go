package steps

import (
	api_v1 "k8s.io/api/core/v1"
	"k8s.io/ingress-gce/pkg/composite"
)

// Step represents single transformation of Forwarding Rules.
// Multiple steps are chained to achieve desired transformation.
//
// Forwarding Rules slice is passed to each step and it's items are mutated.
// Since some steps need to change the slice itself, the function returns the slice.
type Step func(ports []api_v1.ServicePort, frs []*composite.ForwardingRule) ([]*composite.ForwardingRule, error)
