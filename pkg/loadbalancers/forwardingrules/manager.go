package forwardingrules

import (
	cloudprovider "github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud"
	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/filter"
	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	"google.golang.org/api/compute/v1"

	"k8s.io/client-go/tools/record"
	"k8s.io/cloud-provider-gcp/providers/gce"
)

const (
	// Fieldname 'name' for forwarding rules
	nameField = "name"
	// L4 is always regional
	scope = meta.Regional
)

type L4ForwardingManagerConfig struct {
	Namespace string
	Name string
}

type L4ForwardingManager struct {
	gceCloud *gce.Cloud
	namer L4ResourcesNamer
	cfg L4ForwardingManagerConfig

	recorder record.EventRecorder
}

func (l4fm *L4ForwardingManager) Ensure() error {
	return nil
}

func (l4fm *L4ForwardingManager) GetCurrentIPv4ForwardingRules() ([]*compute.ForwardingRule, error) {
	ctx, cancel := cloudprovider.ContextWithCallTimeout()
	defer cancel()
	// Might need to be filled
	var region string

	filter := l4fm.getIPv4Filter()
	// Not sure how to use composite with filters
	return l4fm.gceCloud.Compute().ForwardingRules().List(ctx, region, filter)
}

func (l4fm *L4ForwardingManager) getIPv4Filter() *filter.F {
	tcpName := l4fm.namer.L4ForwardingRule(l4fm.cfg.Namespace, l4fm.cfg.Name, "tcp")
	udpName := l4fm.namer.L4ForwardingRule(l4fm.cfg.Namespace, l4fm.cfg.Name, "udp")
	// L3_Default is not currently supported so we can ignore it here.
	
	// OR
	joinedRegex := tcpName + "|" + udpName
	return filter.Regexp(nameField, joinedRegex)
}

func (l4fm *L4ForwardingManager) DeleteIPv4ForwardingRule() error {
	ctx, cancel := cloudprovider.ContextWithCallTimeout()
	defer cancel()

	key := &meta.Key{
	}
	return l4fm.gceCloud.Compute().ForwardingRules().Delete(ctx, key)
}

