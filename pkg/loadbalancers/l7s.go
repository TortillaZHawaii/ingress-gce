/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package loadbalancers

import (
	"fmt"
	"net/http"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/filter"
	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/cloud-provider-gcp/providers/gce"
	"k8s.io/ingress-gce/pkg/common/operator"
	"k8s.io/ingress-gce/pkg/composite"
	"k8s.io/ingress-gce/pkg/events"
	"k8s.io/ingress-gce/pkg/loadbalancers/features"
	"k8s.io/ingress-gce/pkg/utils"
	"k8s.io/ingress-gce/pkg/utils/common"
	namer_util "k8s.io/ingress-gce/pkg/utils/namer"
	"k8s.io/klog/v2"
)

// L7s implements LoadBalancerPool.
type L7s struct {
	cloud *gce.Cloud
	// v1NamerHelper is an interface for helper functions for v1 frontend naming scheme.
	v1NamerHelper    namer_util.V1FrontendNamer
	recorderProducer events.RecorderProducer
	// namerFactory creates frontend naming policy for ingress/ load balancer.
	namerFactory namer_util.IngressFrontendNamerFactory

	logger klog.Logger
}

// NewLoadBalancerPool returns a new loadbalancer pool.
// - cloud: implements LoadBalancers. Used to sync L7 loadbalancer resources
//
//	with the cloud.
func NewLoadBalancerPool(cloud *gce.Cloud, v1NamerHelper namer_util.V1FrontendNamer, recorderProducer events.RecorderProducer, namerFactory namer_util.IngressFrontendNamerFactory, logger klog.Logger) LoadBalancerPool {
	return &L7s{
		cloud:            cloud,
		v1NamerHelper:    v1NamerHelper,
		recorderProducer: recorderProducer,
		namerFactory:     namerFactory,
		logger:           logger.WithName("L7Pool"),
	}
}

// Ensure implements LoadBalancerPool.
func (l7s *L7s) Ensure(ri *L7RuntimeInfo) (*L7, error) {
	lb := &L7{
		runtimeInfo: ri,
		cloud:       l7s.cloud,
		namer:       l7s.namerFactory.Namer(ri.Ingress),
		recorder:    l7s.recorderProducer.Recorder(ri.Ingress.Namespace),
		scope:       features.ScopeFromIngress(ri.Ingress),
		ingress:     *ri.Ingress,
		logger:      l7s.logger,
	}

	if !lb.namer.IsValidLoadBalancer() {
		err := fmt.Errorf("invalid loadbalancer name %s, the resource name must comply with RFC1035 (https://www.ietf.org/rfc/rfc1035.txt)", lb.namer.LoadBalancer())
		l7s.logger.Error(err, "invalid loadbalancer")
		return nil, err
	}

	if err := lb.edgeHop(); err != nil {
		return nil, fmt.Errorf("loadbalancer %v does not exist: %v", lb.String(), err)
	}
	return lb, nil
}

// delete deletes a loadbalancer by frontend namer.
func (l7s *L7s) delete(namer namer_util.IngressFrontendNamer, versions *features.ResourceVersions, scope meta.KeyType) error {
	if !namer.IsValidLoadBalancer() {
		l7s.logger.V(2).Info("Loadbalancer name invalid, skipping GC", "name", namer.LoadBalancer())
		return nil
	}
	lb := &L7{
		runtimeInfo: &L7RuntimeInfo{},
		cloud:       l7s.cloud,
		namer:       namer,
		scope:       scope,
		logger:      l7s.logger,
	}

	l7s.logger.V(2).Info("Deleting loadbalancer", "name", lb.String())

	if err := lb.Cleanup(versions); err != nil {
		return err
	}
	return nil
}

// list returns a list of urlMaps (the top level LB resource) that belong to the cluster.
func (l7s *L7s) list(key *meta.Key, version meta.Version) ([]*composite.UrlMap, error) {
	var result []*composite.UrlMap
	urlMaps, err := composite.ListUrlMaps(l7s.cloud, key, version, l7s.logger, filter.None)
	if err != nil {
		return nil, err
	}

	for _, um := range urlMaps {
		if l7s.v1NamerHelper.NameBelongsToCluster(um.Name) {
			result = append(result, um)
		}
	}

	return result, nil
}

// GCv2 implements LoadBalancerPool.
func (l7s *L7s) GCv2(ing *v1.Ingress, scope meta.KeyType) error {
	ingKey := common.NamespacedName(ing)
	l7s.logger.V(2).Info("GCv2", "key", ingKey)
	if err := l7s.delete(l7s.namerFactory.Namer(ing), features.VersionsFromIngress(ing), scope); err != nil {
		return err
	}
	l7s.logger.V(2).Info("GCv2 ok", "key", ingKey)
	return nil
}

// FrontendScopeChangeGC returns the scope to GC if the LB has changed scopes
// (e.g. when a user migrates from ILB to ELB on the same ingress or vice versa.)
// This only applies to the V2 Naming Scheme
// TODO(shance): Refactor to avoid calling GCE every sync loop
func (l7s *L7s) FrontendScopeChangeGC(ing *v1.Ingress, ingLogger klog.Logger) (*meta.KeyType, error) {
	if ing == nil {
		return nil, nil
	}

	namer := l7s.namerFactory.Namer(ing)
	urlMapName := namer.UrlMap()
	currentScope := features.ScopeFromIngress(ing)

	for _, scope := range []meta.KeyType{meta.Global, meta.Regional} {
		if scope != currentScope {
			key, err := composite.CreateKey(l7s.cloud, urlMapName, scope)
			if err != nil {
				return nil, err
			}

			// Look for existing LBs with the same name but of a different scope
			_, err = composite.GetUrlMap(l7s.cloud, key, features.VersionsFromIngress(ing).UrlMap, l7s.logger)
			if err == nil {
				l7s.logger.V(2).Info("GC'ing ing for scope", "ing", ing, "scope", scope)
				return &scope, nil
			}
			if !utils.IsHTTPErrorCode(err, http.StatusNotFound) {
				return nil, err
			}
		}
	}
	return nil, nil
}

// DidRegionalClassChange detects if regional ingress changed type between ILB and RXLB.
// We should garbage collect frontend resources on such change, because RXLB and ILB
// use the same name, but different LoadBalancingScheme.
func (l7s *L7s) DidRegionalClassChange(ing *v1.Ingress, ingLogger klog.Logger) (bool, error) {
	if ing == nil {
		return false, nil
	}

	namer := l7s.namerFactory.Namer(ing)
	currentLBScheme := lbSchemeForIngress(ing)
	ingLogger.WithName("DidRegionalClassChange")
	ingLogger.Info("Checking ingress for class name change")

	for _, protocol := range []namer_util.NamerProtocol{namer_util.HTTPProtocol, namer_util.HTTPSProtocol} {
		frName := namer.ForwardingRule(protocol)

		key, err := composite.CreateKey(l7s.cloud, frName, meta.Regional)
		if err != nil {
			return false, err
		}
		ingLogger.Info("Checking for existence of forwarding rule with different LoadBalancingScheme", "frKey", key)

		fr, err := composite.GetForwardingRule(l7s.cloud, key, features.VersionsFromIngress(ing).ForwardingRule, ingLogger)
		if err == nil && fr.LoadBalancingScheme != currentLBScheme {
			ingLogger.Info("ingress needs GC for changed lb scheme", "ingress", ing, "schemeToClean", fr.LoadBalancingScheme)
			return true, nil
		}
		if !utils.IsHTTPErrorCode(err, http.StatusNotFound) {
			return false, err
		}
	}
	return false, nil
}

func lbSchemeForIngress(ing *v1.Ingress) string {
	if utils.IsGCEL7XLBRegionalIngress(ing) {
		return "EXTERNAL_MANAGED"
	} else if utils.IsGCEL7ILBIngress(ing) {
		return "INTERNAL_MANAGED"
	} else {
		return "EXTERNAL"
	}
}

// GCv1 implements LoadBalancerPool.
// TODO(shance): Update to handle regional and global LB with same name
func (l7s *L7s) GCv1(names []string) error {
	l7s.logger.V(2).Info("GCv1", "names", names)

	knownLoadBalancers := make(map[namer_util.LoadBalancerName]bool)
	for _, n := range names {
		knownLoadBalancers[l7s.v1NamerHelper.LoadBalancer(n)] = true
	}

	// GC L7-ILB LBs if enabled
	key, err := composite.CreateKey(l7s.cloud, "", meta.Regional)
	if err != nil {
		return fmt.Errorf("error getting regional key: %v", err)
	}
	urlMaps, err := l7s.list(key, features.L7ILBVersions().UrlMap)
	if err != nil {
		return fmt.Errorf("error listing regional LBs: %v", err)
	}

	if err := l7s.gc(urlMaps, knownLoadBalancers, features.L7ILBVersions()); err != nil {
		return fmt.Errorf("error gc-ing regional LBs: %v", err)
	}

	// TODO(shance): fix list taking a key
	urlMaps, err = l7s.list(meta.GlobalKey(""), meta.VersionGA)
	if err != nil {
		return fmt.Errorf("error listing global LBs: %v", err)
	}

	if errors := l7s.gc(urlMaps, knownLoadBalancers, features.GAResourceVersions); errors != nil {
		return fmt.Errorf("error gcing global LBs: %v", errors)
	}

	return nil
}

// gc is a helper for GCv1.
// TODO(shance): get versions from description
func (l7s *L7s) gc(urlMaps []*composite.UrlMap, knownLoadBalancers map[namer_util.LoadBalancerName]bool, versions *features.ResourceVersions) []error {
	var errors []error

	// Delete unknown loadbalancers
	for _, um := range urlMaps {
		l7Name := l7s.v1NamerHelper.LoadBalancerForURLMap(um.Name)

		if knownLoadBalancers[l7Name] {
			l7s.logger.V(3).Info("Load balancer is still valid, not GC'ing", "name", l7Name)
			continue
		}

		scope, err := composite.ScopeFromSelfLink(um.SelfLink)
		if err != nil {
			errors = append(errors, fmt.Errorf("error getting scope from self link for urlMap %v: %v", um, err))
			continue
		}

		if err := l7s.delete(l7s.namerFactory.NamerForLoadBalancer(l7Name), versions, scope); err != nil {
			errors = append(errors, fmt.Errorf("error deleting loadbalancer %q: %v", l7Name, err))
		}
	}
	return errors
}

// Shutdown implements LoadBalancerPool.
func (l7s *L7s) Shutdown(ings []*v1.Ingress) error {
	// Delete ingresses that use v1 naming scheme.
	if err := l7s.GCv1([]string{}); err != nil {
		return fmt.Errorf("error deleting load-balancers for v1 naming policy: %v", err)
	}
	// Delete ingresses that use v2 naming policy.
	var errs []error
	v2Ings := operator.Ingresses(ings).Filter(func(ing *v1.Ingress) bool {
		return namer_util.FrontendNamingScheme(ing, l7s.logger) == namer_util.V2NamingScheme
	}).AsList()
	for _, ing := range v2Ings {
		if err := l7s.GCv2(ing, features.ScopeFromIngress(ing)); err != nil {
			errs = append(errs, err)
		}
	}
	if errs != nil {
		return fmt.Errorf("error deleting load-balancers for v2 naming policy: %v", utils.JoinErrs(errs))
	}
	l7s.logger.V(2).Info("Loadbalancer pool shutdown.")
	return nil
}

// HasUrlMap implements LoadBalancerPool.
func (l7s *L7s) HasUrlMap(ing *v1.Ingress) (bool, error) {
	namer := l7s.namerFactory.Namer(ing)
	key, err := composite.CreateKey(l7s.cloud, namer.UrlMap(), features.ScopeFromIngress(ing))
	if err != nil {
		return false, err
	}
	if _, err := composite.GetUrlMap(l7s.cloud, key, features.VersionsFromIngress(ing).UrlMap, l7s.logger); err != nil {
		if utils.IsHTTPErrorCode(err, http.StatusNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
