package forwardingrules

import (
	"github.com/go-logr/logr"
	api_v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/cloud-provider-gcp/providers/gce"
	"k8s.io/ingress-gce/pkg/address"
	"k8s.io/ingress-gce/pkg/composite"
	"k8s.io/ingress-gce/pkg/utils"
)

// NetLBSetsManager will manage a set of Forwarding Rules for NetLB.
type NetLBSetsManager struct {
	Namer    Namer
	Provider Provider
	Recorder record.EventRecorder
	Logger   logr.Logger
	Cloud    *gce.Cloud

	Service *api_v1.Service
}

type EnsureResult struct {
	ForwardingRules []*composite.ForwardingRule
	SyncStatus      utils.ResourceSyncStatus
	IPManaged       address.IPAddressType
}

// EnsureIPv4 creates or updates existing set of Forwarding Rules based on the Service definition.
func (m *NetLBSetsManager) EnsureIPv4(backendServiceLink string) (EnsureResult, error) {
	// log := m.Logger.WithName("EnsureIPv4")
	return EnsureResult{}, nil
}

// ListIPv4 returns all of the GCE Forwarding Rules managed for the LB.
func (m *NetLBSetsManager) ListIPv4(backendServiceLink string) ([]*composite.ForwardingRule, error) {
	// log := m.Logger.WithName("GetIPv4")

	return nil, nil
}

//
