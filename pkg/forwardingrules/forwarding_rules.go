package forwardingrules

import (
	"encoding/json"
	"fmt"

	cloudprovider "github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud"
	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/filter"
	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	"k8s.io/cloud-provider-gcp/providers/gce"
	"k8s.io/ingress-gce/pkg/composite"
	compositemetrics "k8s.io/ingress-gce/pkg/composite/metrics"
	"k8s.io/ingress-gce/pkg/utils"
	"k8s.io/klog/v2"
)

type ForwardingRules struct {
	cloud   *gce.Cloud
	version meta.Version
	scope   meta.KeyType

	logger klog.Logger
}

func New(cloud *gce.Cloud, version meta.Version, scope meta.KeyType, logger klog.Logger) *ForwardingRules {
	return &ForwardingRules{
		cloud:   cloud,
		version: version,
		scope:   scope,
		logger:  logger.WithName("ForwardingRules"),
	}
}

func (frc *ForwardingRules) Create(forwardingRule *composite.ForwardingRule) error {
	key, err := frc.createKey(forwardingRule.Name)
	if err != nil {
		frc.logger.Error(err, "Failed to create key for creating forwarding rule", "forwardingRuleName", forwardingRule.Name)
		return nil
	}
	return composite.CreateForwardingRule(frc.cloud, key, forwardingRule, frc.logger)
}

func (frc *ForwardingRules) Patch(forwardingRule *composite.ForwardingRule) error {
	key, err := frc.createKey(forwardingRule.Name)
	if err != nil {
		frc.logger.Error(err, "Failed to create key for creating forwarding rule", "forwardingRuleName", forwardingRule.Name)
		return nil
	}
	return composite.PatchForwardingRule(frc.cloud, key, forwardingRule, frc.logger)
}

func (frc *ForwardingRules) Get(name string) (*composite.ForwardingRule, error) {
	key, err := frc.createKey(name)
	if err != nil {
		return nil, fmt.Errorf("Failed to create key for fetching forwarding rule %s, err: %w", name, err)
	}
	fr, err := composite.GetForwardingRule(frc.cloud, key, frc.version, frc.logger)
	if utils.IgnoreHTTPNotFound(err) != nil {
		return nil, fmt.Errorf("Failed to get existing forwarding rule %s, err: %w", name, err)
	}
	return fr, nil
}

// List will list all of the Forwarding Rules in GCE matching the filter.
//
// ListForwardingRules in pkg/composite/gen.go doesn't let us pass filters which necessitates
// copying most of the code from there and adding an option to pass filter.
func (frc *ForwardingRules) List(filter *filter.F) ([]*composite.ForwardingRule, error) {
	key, err := frc.createKey("")
	if err != nil {
		return nil, fmt.Errorf("failed to create key for listing forwarding rules, err: %w", err)
	}

	// based on pkg/composite/gen.go/ListForwardingRules
	// however ListForwardingRules doesn't allow passing in filters,
	// which we need to use regular expressions to find Forwarding Rules
	// only for specific LBs.
	logger := frc.logger.WithName("List")
	ctx, cancel := cloudprovider.ContextWithCallTimeout()
	defer cancel()
	mc := compositemetrics.NewMetricContext("ForwardingRule", "list", key.Region, key.Zone, string(frc.version))

	var gceObjs interface{}
	switch frc.version {
	case meta.VersionAlpha:
		switch key.Type() {
		case meta.Regional:
			logger.Info("Listing alpha region ForwardingRule")
			gceObjs, err = frc.cloud.Compute().AlphaForwardingRules().List(ctx, key.Region, filter)
		default:
			logger.Info("Listing alpha ForwardingRule")
			gceObjs, err = frc.cloud.Compute().AlphaGlobalForwardingRules().List(ctx, filter)
		}
	case meta.VersionBeta:
		switch key.Type() {
		case meta.Regional:
			logger.Info("Listing beta region ForwardingRule")
			gceObjs, err = frc.cloud.Compute().BetaForwardingRules().List(ctx, key.Region, filter)
		default:
			logger.Info("Listing beta ForwardingRule")
			gceObjs, err = frc.cloud.Compute().BetaGlobalForwardingRules().List(ctx, filter)
		}
	default:
		switch key.Type() {
		case meta.Regional:
			logger.Info("Listing ga region ForwardingRule")
			gceObjs, err = frc.cloud.Compute().ForwardingRules().List(ctx, key.Region, filter)
		default:
			logger.Info("Listing ga ForwardingRule")
			gceObjs, err = frc.cloud.Compute().GlobalForwardingRules().List(ctx, filter)
		}
	}
	err = mc.Observe(err)
	if err != nil {
		return nil, err
	}

	compositeObjs, err := toForwardingRuleList(gceObjs)
	if err != nil {
		return nil, err
	}
	for _, obj := range compositeObjs {
		obj.Version = frc.version
	}
	return compositeObjs, nil
}

func (frc *ForwardingRules) Delete(name string) error {
	key, err := frc.createKey(name)
	if err != nil {
		return fmt.Errorf("Failed to create key for deleting forwarding rule %s, err: %w", name, err)
	}
	err = composite.DeleteForwardingRule(frc.cloud, key, frc.version, frc.logger)
	if utils.IgnoreHTTPNotFound(err) != nil {
		return fmt.Errorf("Failed to delete forwarding rule %s, err: %w", name, err)
	}
	return nil
}

func (frc *ForwardingRules) createKey(name string) (*meta.Key, error) {
	return composite.CreateKey(frc.cloud, name, frc.scope)
}

// toForwardingRuleList converts a list of compute alpha, beta or GA
// ForwardingRule into a list of our composite type.
func toForwardingRuleList(objs interface{}) ([]*composite.ForwardingRule, error) {
	result := []*composite.ForwardingRule{}

	err := copyViaJSON(&result, objs)
	if err != nil {
		return nil, fmt.Errorf("could not copy object %v to %T via JSON: %v", objs, result, err)
	}
	return result, nil
}

func copyViaJSON(dest interface{}, src interface{}) error {
	var err error
	bytes, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, dest)
}
