package step

import (
	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/filter"
	"k8s.io/ingress-gce/pkg/composite"
)

// Namer is used to get names for forwarding rules.
type Namer interface {
	L4ForwardingRule(namespace, name, protocol string) string
}

// Provider is the interface for the ForwardingRules provider.
// We can't use the *ForwardingRules directly, since L4NetLB uses interface.
//
// It is assumed that delete doesn't return 404 errors when forwarding rule doesn't exist.
type Provider interface {
	Get(name string) (*composite.ForwardingRule, error)
	Create(forwardingRule *composite.ForwardingRule) error
	Delete(name string) error
	Patch(forwardingRule *composite.ForwardingRule) error
	List(filter *filter.F) ([]*composite.ForwardingRule, error)
}
