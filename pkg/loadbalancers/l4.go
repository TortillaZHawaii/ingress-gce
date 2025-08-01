/*
Copyright 2020 The Kubernetes Authors.

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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud"
	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	"google.golang.org/api/compute/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/cloud-provider-gcp/providers/gce"
	"k8s.io/cloud-provider/service/helpers"
	"k8s.io/ingress-gce/pkg/address"
	"k8s.io/ingress-gce/pkg/annotations"
	"k8s.io/ingress-gce/pkg/backends"
	"k8s.io/ingress-gce/pkg/composite"
	"k8s.io/ingress-gce/pkg/firewalls"
	"k8s.io/ingress-gce/pkg/flags"
	"k8s.io/ingress-gce/pkg/forwardingrules"
	"k8s.io/ingress-gce/pkg/healthchecksl4"
	"k8s.io/ingress-gce/pkg/metrics"
	"k8s.io/ingress-gce/pkg/network"
	"k8s.io/ingress-gce/pkg/utils"
	"k8s.io/ingress-gce/pkg/utils/namer"
	"k8s.io/klog/v2"
)

const (
	subnetInternalIPv6AccessType          = "INTERNAL"
	trafficDistribuitionZonalAffinity     = "PreferClose"
	WeightedLBPodsPerNodeAllowlistMessage = "Weighted Load Balancing for L4 " +
		"Internal Passthrough Load Balancers requires project allowlisting. If " +
		"you need access to this feature please contact Google Cloud support team"
)

var noConnectionTrackingPolicy *composite.BackendServiceConnectionTrackingPolicy = nil

// Many of the functions in this file are re-implemented from gce_loadbalancer_internal.go
// L4 handles the resource creation/deletion/update for a given L4 ILB service.
type L4 struct {
	cloud       *gce.Cloud
	backendPool *backends.Pool
	scope       meta.KeyType
	namer       namer.L4ResourcesNamer
	// recorder is used to generate k8s Events.
	recorder                         record.EventRecorder
	Service                          *corev1.Service
	ServicePort                      utils.ServicePort
	NamespacedName                   types.NamespacedName
	forwardingRules                  ForwardingRulesProvider
	healthChecks                     healthchecksl4.L4HealthChecks
	enableDualStack                  bool
	network                          network.NetworkInfo
	networkResolver                  network.Resolver
	enableWeightedLB                 bool
	enableMixedProtocol              bool
	disableNodesFirewallProvisioning bool
	enableZonalAffinity              bool
	svcLogger                        klog.Logger
	configMapLister                  cache.Store
}

// L4ILBSyncResult contains information about the outcome of an L4 ILB sync. It stores the list of resource name annotations,
// sync error, the GCE resource that hit the error along with the error type, metrics and more fields.
type L4ILBSyncResult struct {
	Annotations        map[string]string
	Error              error
	GCEResourceInError string
	Status             *corev1.LoadBalancerStatus
	MetricsLegacyState metrics.L4ILBServiceLegacyState
	MetricsState       metrics.L4ServiceState
	SyncType           string
	StartTime          time.Time
	ResourceUpdates    ResourceUpdates
}

func NewL4ILBSyncResult(syncType string, startTime time.Time, svc *corev1.Service, isMultinetService bool, isWeightedLBPodsPerNode bool, isLBWithZonalAffinity bool) *L4ILBSyncResult {
	enabledStrongSessionAffinity := false
	result := &L4ILBSyncResult{
		Annotations: make(map[string]string),
		StartTime:   startTime,
		SyncType:    syncType,
		// Internal Load Balancer doesn't support strong session affinity (passing `false` all along)
		MetricsState: metrics.InitServiceMetricsState(svc, &startTime, isMultinetService, enabledStrongSessionAffinity, isWeightedLBPodsPerNode, isLBWithZonalAffinity, metrics.L4BackendTypeNEG),
	}
	return result
}

type L4ILBParams struct {
	Service                          *corev1.Service
	Cloud                            *gce.Cloud
	Namer                            namer.L4ResourcesNamer
	Recorder                         record.EventRecorder
	DualStackEnabled                 bool
	NetworkResolver                  network.Resolver
	EnableWeightedLB                 bool
	EnableZonalAffinity              bool
	DisableNodesFirewallProvisioning bool
	EnableMixedProtocol              bool
	ConfigMapLister                  cache.Store
}

// NewL4Handler creates a new L4Handler for the given L4 service.
func NewL4Handler(params *L4ILBParams, logger klog.Logger) *L4 {
	logger = logger.WithName("L4Handler")

	var scope meta.KeyType = meta.Regional
	l4 := &L4{
		cloud:                            params.Cloud,
		scope:                            scope,
		namer:                            params.Namer,
		recorder:                         params.Recorder,
		Service:                          params.Service,
		healthChecks:                     healthchecksl4.NewL4HealthChecks(params.Cloud, params.Recorder, logger),
		forwardingRules:                  forwardingrules.New(params.Cloud, meta.VersionGA, scope, logger),
		enableDualStack:                  params.DualStackEnabled,
		networkResolver:                  params.NetworkResolver,
		enableWeightedLB:                 params.EnableWeightedLB,
		enableMixedProtocol:              params.EnableMixedProtocol,
		disableNodesFirewallProvisioning: params.DisableNodesFirewallProvisioning,
		enableZonalAffinity:              params.EnableZonalAffinity,
		svcLogger:                        logger,
		configMapLister:                  params.ConfigMapLister,
	}
	l4.NamespacedName = types.NamespacedName{Name: params.Service.Name, Namespace: params.Service.Namespace}
	l4.backendPool = backends.NewPool(l4.cloud, l4.namer)
	l4.ServicePort = utils.ServicePort{
		ID: utils.ServicePortID{Service: l4.NamespacedName}, BackendNamer: l4.namer,
		VMIPNEGEnabled: true,
	}
	return l4
}

// CreateKey generates a meta.Key for a given GCE resource name.
func (l4 *L4) CreateKey(name string) (*meta.Key, error) {
	return composite.CreateKey(l4.cloud, name, l4.scope)
}

// getILBOptions fetches the optional features requested on the given ILB service.
func (l4 *L4) getILBOptions() gce.ILBOptions {
	if l4.cloud.IsLegacyNetwork() {
		l4.recorder.Event(l4.Service, corev1.EventTypeWarning, "ILBOptionsIgnored", "Internal LoadBalancer options are not supported with Legacy Networks.")
		return gce.ILBOptions{}
	}

	return gce.ILBOptions{
		AllowGlobalAccess: gce.GetLoadBalancerAnnotationAllowGlobalAccess(l4.Service),
		SubnetName:        annotations.FromService(l4.Service).GetInternalLoadBalancerAnnotationSubnet(),
	}
}

// EnsureInternalLoadBalancerDeleted performs a cleanup of all GCE resources for the given loadbalancer service.
func (l4 *L4) EnsureInternalLoadBalancerDeleted(svc *corev1.Service) *L4ILBSyncResult {
	l4.svcLogger.V(2).Info("EnsureInternalLoadBalancerDeleted: deleting L4 ILB LoadBalancer resources")
	isMultinetService := l4.networkResolver.IsMultinetService(svc)
	isWeightedLBPodsPerNode := l4.isWeightedLBPodsPerNode()
	isLBWithZonalAffinity := l4.isLBWithZonalAffinity()
	result := NewL4ILBSyncResult(SyncTypeDelete, time.Now(), svc, isMultinetService, isWeightedLBPodsPerNode, isLBWithZonalAffinity)

	l4.deleteIPv4ResourcesOnDelete(result)
	if l4.enableDualStack {
		l4.deleteIPv6ResourcesOnDelete(result)
	}

	// Delete backend service
	bsName := l4.namer.L4Backend(svc.Namespace, svc.Name)
	// TODO(cheungdavid): Create backend logger that contains backendName,
	// backendVersion, and backendScope before passing to backendPool.Delete().
	// See example in backendSyncer.gc().
	err := utils.IgnoreHTTPNotFound(l4.backendPool.Delete(bsName, meta.VersionGA, meta.Regional, l4.svcLogger))
	if err != nil {
		l4.svcLogger.Error(err, "Failed to delete backends for internal loadbalancer service")
		result.GCEResourceInError = annotations.BackendServiceResource
		result.Error = err
	}

	// Delete healthcheck
	// We don't delete health check during service update so
	// it is possible that there might be some health check leak
	// when externalTrafficPolicy is changed from Local to Cluster and a new health check was created.
	// When service is deleted we need to check both health checks shared and non-shared
	// and delete them if needed.
	for _, isShared := range []bool{true, false} {
		if l4.enableDualStack {
			resourceInError, err := l4.healthChecks.DeleteHealthCheckWithDualStackFirewalls(svc, l4.namer, isShared, meta.Global, utils.ILB, l4.svcLogger)
			if err != nil {
				result.GCEResourceInError = resourceInError
				result.Error = err
			}
		} else {
			resourceInError, err := l4.healthChecks.DeleteHealthCheckWithFirewall(svc, l4.namer, isShared, meta.Global, utils.ILB, l4.svcLogger)
			if err != nil {
				result.GCEResourceInError = resourceInError
				result.Error = err
			}
		}
	}
	return result
}

// deleteIPv4ResourcesOnSync deletes resources specific to IPv4,
// only if corresponding resource annotation exist on Service object.
// This function is called only on Service update or periodic sync.
// Checking for annotation saves us from emitting too much error logs "Resource not found".
// If annotation was deleted, but resource still exists, it will be left till the Service deletion,
// where we delete all resources, no matter if they exist in annotations.
func (l4 *L4) deleteIPv4ResourcesOnSync(result *L4ILBSyncResult) {
	l4.svcLogger.Info("Deleting IPv4 resources for L4 ILB Service on sync, with checking for existence in annotation")
	l4.deleteIPv4ResourcesAnnotationBased(result, false)
}

// deleteIPv4ResourcesOnDelete deletes all resources specific to IPv4.
// This function is called only on Service deletion.
// During sync, we delete resources only that exist in annotations,
// so they could be leaked, if annotation was deleted.
// That's why on service deletion we delete all IPv4 resources, ignoring their existence in annotations
func (l4 *L4) deleteIPv4ResourcesOnDelete(result *L4ILBSyncResult) {
	l4.svcLogger.Info("Deleting IPv4 resources for L4 ILB Service on delete, without checking for existence in annotation")
	l4.deleteIPv4ResourcesAnnotationBased(result, true)
}

// deleteIPv4ResourcesAnnotationBased deletes IPv4 only resources with checking,
// if resource exists in Service annotation, if shouldIgnoreAnnotations not set to true
// IPv4 Specific resources:
// - IPv4 Forwarding Rule
// - IPv4 Address
// - IPv4 Firewall
// This function does not delete Backend Service and Health Check, because they are shared between IPv4 and IPv6.
// IPv4 Firewall Rule for Health Check also will not be deleted here, and will be left till the Service Deletion.
func (l4 *L4) deleteIPv4ResourcesAnnotationBased(result *L4ILBSyncResult, shouldIgnoreAnnotations bool) {
	hasFwdRuleAnnotation := l4.hasAnnotation(annotations.TCPForwardingRuleKey) ||
		l4.hasAnnotation(annotations.UDPForwardingRuleKey) ||
		l4.hasAnnotation(annotations.L3ForwardingRuleKey)
	if shouldIgnoreAnnotations || hasFwdRuleAnnotation {
		var err error
		if l4.enableDualStack && l4.enableMixedProtocol {
			err = l4.deleteAllIPv4ForwardingRules()
		} else {
			err = l4.deleteIPv4ForwardingRule()
		}
		if err != nil {
			l4.svcLogger.Error(err, "Failed to delete forwarding rule for internal loadbalancer service")
			result.Error = err
			result.GCEResourceInError = annotations.ForwardingRuleResource
		}
	}

	// Deleting non-existent address do not print error audit logs, and we don't store address in annotations
	// that's why we can delete it without checking annotation
	err := l4.deleteIPv4Address()
	if err != nil {
		l4.svcLogger.Error(err, "Failed to delete address for internal loadbalancer service")
		result.Error = err
		result.GCEResourceInError = annotations.AddressResource
	}

	// delete firewall rule allowing load balancer source ranges
	if shouldIgnoreAnnotations || l4.hasAnnotation(annotations.FirewallRuleKey) {
		err := l4.deleteIPv4NodesFirewall()
		if err != nil {
			l4.svcLogger.Error(err, "Failed to delete firewall rule for internal loadbalancer service")
			result.GCEResourceInError = annotations.FirewallRuleResource
			result.Error = err
		}
	}
}

func (l4 *L4) deleteIPv4ForwardingRule() error {
	start := time.Now()

	frName := l4.GetFRName()

	l4.svcLogger.Info("Deleting IPv4 forwarding rule for L4 ILB Service", "forwardingRuleName", frName)
	defer func() {
		l4.svcLogger.Info("Finished deleting IPv4 forwarding rule for L4 ILB Service", "forwardingRuleName", frName, "timeTaken", time.Since(start))
	}()

	return l4.forwardingRules.Delete(frName)
}

// When switching from for example IPv4 TCP to IPv6 UDP, simply using GetFRName()
// to delete won't be accurate since it will try to delete *IPv4 UDP* name not *TCP*,
// and the UDP forwarding rule will be leaked.
func (l4 *L4) deleteAllIPv4ForwardingRules() error {
	start := time.Now()

	frNameTCP := l4.getFRNameWithProtocol(forwardingrules.ProtocolTCP)
	frNameUDP := l4.getFRNameWithProtocol(forwardingrules.ProtocolUDP)
	frNameL3 := l4.getFRNameWithProtocol(forwardingrules.ProtocolL3)

	l4.svcLogger.Info("Deleting all potential IPv4 forwarding rule for L4 ILB Service",
		"forwardingRuleNameTCP", frNameTCP, "forwardingRuleNameUDP", frNameUDP, "forwardingRuleNameL3", frNameL3)
	defer func() {
		l4.svcLogger.Info("Finished deleting all IPv4 forwarding rule for L4 ILB Service", "forwardingRuleNameTCP", frNameTCP, "forwardingRuleNameUDP", frNameUDP, "forwardingRuleNameL3", frNameL3, "timeTaken", time.Since(start))
	}()

	errTCP := l4.forwardingRules.Delete(frNameTCP)
	errUDP := l4.forwardingRules.Delete(frNameUDP)
	errL3 := l4.forwardingRules.Delete(frNameL3)

	return errors.Join(errTCP, errUDP, errL3)
}

func (l4 *L4) deleteIPv4Address() error {
	addressName := l4.GetFRName()

	start := time.Now()
	l4.svcLogger.Info("Deleting IPv4 address for L4 ILB Service", "addressName", addressName)
	defer func() {
		l4.svcLogger.Info("Finished deleting IPv4 address for L4 ILB Service", "addressName", addressName, "timeTaken", time.Since(start))
	}()

	return address.EnsureDeleted(l4.cloud, addressName, l4.cloud.Region())
}

func (l4 *L4) deleteIPv4NodesFirewall() error {
	start := time.Now()

	firewallName := l4.namer.L4Firewall(l4.Service.Namespace, l4.Service.Name)

	fwLogger := l4.svcLogger.WithValues("firewallName", firewallName)
	fwLogger.Info("Deleting IPv4 nodes firewall for L4 ILB Service")
	defer func() {
		fwLogger.Info("Finished deleting IPv4 nodes firewall for L4 ILB Service", "timeTaken", time.Since(start))
	}()

	return l4.deleteFirewall(firewallName, fwLogger)
}

func (l4 *L4) deleteFirewall(name string, fwLogger klog.Logger) error {
	err := firewalls.EnsureL4FirewallRuleDeleted(l4.cloud, name, fwLogger)
	if err != nil {
		if fwErr, ok := err.(*firewalls.FirewallXPNError); ok {
			l4.recorder.Eventf(l4.Service, corev1.EventTypeNormal, "XPN", fwErr.Message)
			return nil
		}
		return err
	}
	return nil
}

// GetFRName returns the name of the forwarding rule for the given ILB service.
// This appends the protocol to the forwarding rule name, which will help supporting multiple protocols in the same ILB
// service.
func (l4 *L4) GetFRName() string {
	ports := l4.Service.Spec.Ports
	protocol := string(utils.GetProtocol(ports))
	if l4.enableMixedProtocol {
		protocol = forwardingrules.GetILBProtocol(ports)
	}
	return l4.getFRNameWithProtocol(protocol)
}

func (l4 *L4) getFRNameWithProtocol(protocol string) string {
	return l4.namer.L4ForwardingRule(l4.Service.Namespace, l4.Service.Name, strings.ToLower(protocol))
}

func (l4 *L4) subnetName() string {
	// At first check custom subnet annotation.
	customSubnetName := annotations.FromService(l4.Service).GetInternalLoadBalancerAnnotationSubnet()
	if customSubnetName != "" {
		return customSubnetName
	}

	// If no custom subnet in annotation -- use cluster subnet.
	clusterSubnetURL := l4.cloud.SubnetworkURL()
	splitURL := strings.Split(clusterSubnetURL, "/")
	return splitURL[len(splitURL)-1]
}

// EnsureInternalLoadBalancer ensures that all GCE resources for the given loadbalancer service have
// been created. It returns a LoadBalancerStatus with the updated ForwardingRule IP address.
func (l4 *L4) EnsureInternalLoadBalancer(nodeNames []string, svc *corev1.Service) *L4ILBSyncResult {
	l4.Service = svc

	startTime := time.Now()
	isMultinetService := l4.networkResolver.IsMultinetService(svc)
	isWeightedLBPodsPerNode := l4.isWeightedLBPodsPerNode()
	isWithZonalAffinity := l4.isLBWithZonalAffinity()
	result := NewL4ILBSyncResult(SyncTypeCreate, startTime, svc, isMultinetService, isWeightedLBPodsPerNode, isWithZonalAffinity)

	// If service already has an IP assigned, treat it as an update instead of a new Loadbalancer.
	// This will also cover cases where an external LB is updated to an ILB, which is technically a create for ILB.
	// But this is still the easiest way to identify create vs update in the common case.
	if len(svc.Status.LoadBalancer.Ingress) > 0 {
		result.SyncType = SyncTypeUpdate
	}

	l4.svcLogger.V(2).Info("EnsureInternalLoadBalancer", "syncType", result.SyncType)

	svcNetwork, err := l4.networkResolver.ServiceNetwork(svc)
	if err != nil {
		result.Error = err
		return result
	}
	l4.network = *svcNetwork

	// If service requires IPv6 LoadBalancer -- verify that Subnet with Internal IPv6 ranges is used.
	if l4.enableDualStack && utils.NeedsIPv6(l4.Service) {
		err := l4.serviceSubnetHasInternalIPv6Range()
		if err != nil {
			result.Error = err
			return result
		}
	}

	hcLink := l4.provideHealthChecks(nodeNames, result)
	if result.Error != nil {
		return result
	}

	options := l4.getILBOptions()
	subnetworkURL, err := l4.getServiceSubnetworkURL(options)
	if err != nil {
		result.Error = err
		return result
	}
	l4.svcLogger.V(2).Info("subnetworkURL for service", "subnetworkURL", subnetworkURL)

	bsName := l4.namer.L4Backend(l4.Service.Namespace, l4.Service.Name)
	// TODO(cheungdavid): Create backend logger that contains backendName,
	// backendVersion, and backendScope before passing to backendPool.Get().
	// See example in backendSyncer.ensureBackendService().
	existingBS, err := l4.backendPool.Get(bsName, meta.VersionGA, l4.scope, l4.svcLogger)
	if utils.IgnoreHTTPNotFound(err) != nil {
		l4.svcLogger.Error(err, "Failed to lookup existing backend service, ignoring err")
	}

	// Reserve existing IP address before making any changes
	var existingIPv4FR *composite.ForwardingRule
	var ipv4AddressToUse string
	var ipv4AddressName string
	if !l4.enableDualStack || utils.NeedsIPv4(l4.Service) {
		existingIPv4FR, err = l4.getOldIPv4ForwardingRule(existingBS)
		ipv4AddressToUse, ipv4AddressName, err = address.IPv4ToUse(l4.cloud, l4.recorder, l4.Service, existingIPv4FR, subnetworkURL)
		if err != nil {
			result.Error = fmt.Errorf("EnsureInternalLoadBalancer error: address.IPv4ToUse returned error: %w", err)
			return result
		}

		if !l4.cloud.IsLegacyNetwork() {
			l4.svcLogger.V(2).Info("EnsureInternalLoadBalancer, reserve existing IPv4 address before making any changes")
			nm := types.NamespacedName{Namespace: l4.Service.Namespace, Name: l4.Service.Name}.String()
			// ILB can be created only in Premium Tier
			addrMgr := address.NewManager(l4.cloud, nm, l4.cloud.Region(), subnetworkURL, l4.GetFRName(), ipv4AddressName, ipv4AddressToUse, cloud.SchemeInternal, cloud.NetworkTierPremium, address.IPv4Version, l4.svcLogger)
			ipv4AddressToUse, _, err = addrMgr.HoldAddress()
			if err != nil {
				result.Error = fmt.Errorf("EnsureInternalLoadBalancer error: addrMgr.HoldAddress() returned error %w", err)
				return result
			}
			l4.svcLogger.V(2).Info("EnsureInternalLoadBalancer: reserved IPv4 address", "ipv4AddressToUse", ipv4AddressToUse)
			defer func() {
				// Release the address that was reserved, in all cases. If the forwarding rule was successfully created,
				// the ephemeral IP is not needed anymore. If it was not created, the address should be released to prevent leaks.
				if err := addrMgr.ReleaseAddress(); err != nil {
					l4.svcLogger.Error(err, "EnsureInternalLoadBalancer: failed to release IPv4 address reservation, possibly causing an orphan")
				}
			}()
		}
	}

	// Reserve existing IPv6 address before making any changes
	var existingIPv6FR *composite.ForwardingRule
	var ipv6AddrToUse string
	var ipv6AddressName string
	if l4.enableDualStack && utils.NeedsIPv6(l4.Service) {
		existingIPv6FR, err = l4.getOldIPv6ForwardingRule(existingBS)
		ipv6AddrToUse, ipv6AddressName, err = address.IPv6ToUse(l4.cloud, l4.Service, existingIPv6FR, subnetworkURL, l4.svcLogger)
		if err != nil {
			result.Error = fmt.Errorf("EnsureInternalLoadBalancer error: address.IPv6ToUse returned error: %w", err)
			return result
		}

		l4.svcLogger.V(2).Info("EnsureInternalLoadBalancer, reserve existing IPv6 address before making any changes", "ipAddress", ipv6AddrToUse)

		if !l4.cloud.IsLegacyNetwork() {
			nm := types.NamespacedName{Namespace: l4.Service.Namespace, Name: l4.Service.Name}.String()
			// ILB can be created only in Premium Tier
			ipv6AddrMgr := address.NewManager(l4.cloud, nm, l4.cloud.Region(), subnetworkURL, l4.getIPv6FRName(), ipv6AddressName, ipv6AddrToUse, cloud.SchemeInternal, cloud.NetworkTierPremium, address.IPv6Version, l4.svcLogger)
			ipv6AddrToUse, _, err = ipv6AddrMgr.HoldAddress()
			if err != nil {
				result.Error = fmt.Errorf("EnsureInternalLoadBalancer error: ipv6AddrMgr.HoldAddress() returned error %w", err)
				return result
			}
			l4.svcLogger.V(2).Info("EnsureInternalLoadBalancer: reserved IPv6 address", "ipv6AddressToUse", ipv6AddrToUse)
			defer func() {
				// Release the address that was reserved, in all cases. If the forwarding rule was successfully created,
				// the ephemeral IP is not needed anymore. If it was not created, the address should be released to prevent leaks.
				if err := ipv6AddrMgr.ReleaseAddress(); err != nil {
					l4.svcLogger.Error(err, "EnsureInternalLoadBalancer: failed to release IPv6 address reservation, possibly causing an orphan")
				}
			}()
		}
	}

	servicePorts := l4.Service.Spec.Ports
	backendProtocol := string(utils.GetProtocol(servicePorts))
	if l4.enableMixedProtocol {
		backendProtocol = backends.GetProtocol(servicePorts)
	}

	// if Service protocol changed, we must delete forwarding rule before changing backend service,
	// otherwise, on updating backend service, google cloud api will return error
	if existingBS != nil && existingBS.Protocol != backendProtocol {
		l4.svcLogger.Info("Protocol changed for service", "existingProtocol", existingBS.Protocol, "newProtocol", backendProtocol)
		if existingIPv4FR != nil {
			// Delete ipv4 forwarding rule if it exists
			err = l4.forwardingRules.Delete(existingIPv4FR.Name)
			if err != nil {
				l4.svcLogger.Error(err, "Failed to delete forwarding rule", "forwardingRuleName", existingIPv4FR.Name)
			}
		}

		if l4.enableDualStack && existingIPv6FR != nil {
			// Delete ipv6 forwarding rule if it exists
			err = l4.forwardingRules.Delete(existingIPv6FR.Name)
			if err != nil {
				l4.svcLogger.Error(err, "Failed to delete ipv6 forwarding rule", "forwardingRuleName", existingIPv6FR.Name)
			}
		}
	}

	localityLbPolicy := l4.determineBackendServiceLocalityPolicy()

	enableZonalAffinity := l4.requireZonalAffinity(svc)

	// ensure backend service
	var logConfig *composite.BackendServiceLogConfig
	if flags.F.ManageL4LBLogging && l4.configMapLister != nil {
		logConfig, err = GetL4LoggingConfig(l4.Service, l4.configMapLister)
		if err != nil {
			result.GCEResourceInError = annotations.BackendServiceResource
			result.Error = utils.NewUserError(err)
			return result
		}
	}

	backendParams := backends.L4BackendServiceParams{
		Name:                     bsName,
		HealthCheckLink:          hcLink,
		Protocol:                 backendProtocol,
		SessionAffinity:          string(l4.Service.Spec.SessionAffinity),
		Scheme:                   string(cloud.SchemeInternal),
		NamespacedName:           l4.NamespacedName,
		NetworkInfo:              &l4.network,
		ConnectionTrackingPolicy: noConnectionTrackingPolicy,
		EnableZonalAffinity:      enableZonalAffinity,
		LocalityLbPolicy:         localityLbPolicy,
		LogConfig:                logConfig,
	}

	bs, bsSyncStatus, err := l4.backendPool.EnsureL4BackendService(backendParams, l4.svcLogger)
	result.ResourceUpdates.SetBackendService(bsSyncStatus)
	if err != nil {
		if utils.IsUnsupportedFeatureError(err, string(backends.LocalityLBPolicyMaglev)) {
			result.GCEResourceInError = annotations.BackendServiceResource
			l4.recorder.Eventf(l4.Service, corev1.EventTypeWarning, "AllowlistingRequired", WeightedLBPodsPerNodeAllowlistMessage)
			result.Error = utils.NewUserError(err)
		} else {
			result.GCEResourceInError = annotations.BackendServiceResource
			result.Error = err
		}
		return result
	}
	result.Annotations[annotations.BackendServiceKey] = bsName

	if l4.enableDualStack {
		l4.ensureDualStackResources(result, nodeNames, options, bs, existingIPv4FR, existingIPv6FR, subnetworkURL, ipv4AddressToUse, ipv6AddrToUse)
	} else {
		l4.ensureIPv4Resources(result, nodeNames, options, bs, existingIPv4FR, subnetworkURL, ipv4AddressToUse)
	}
	if result.Error != nil {
		return result
	}

	result.MetricsLegacyState.InSuccess = true
	if options.AllowGlobalAccess {
		result.MetricsLegacyState.EnabledGlobalAccess = true
	}
	// SubnetName is overwritten to nil value if Alpha feature gate for custom subnet
	// is not enabled. So, a non empty subnet name at this point implies that the
	// feature is in use.
	if options.SubnetName != "" {
		result.MetricsLegacyState.EnabledCustomSubnet = true
	}
	result.MetricsState.Status = metrics.StatusSuccess
	result.MetricsState.FirstSyncErrorTime = nil
	return result
}

func (l4 *L4) requireZonalAffinity(svc *corev1.Service) bool {
	return l4.enableZonalAffinity && // zonal affinity flag is enabled
		svc.Spec.TrafficDistribution != nil && // traffic distribution field is set
		*svc.Spec.TrafficDistribution == trafficDistribuitionZonalAffinity // traffic distribution field is set to zonal affinity default "PreferClose"
}

func (l4 *L4) provideHealthChecks(nodeNames []string, result *L4ILBSyncResult) string {
	if l4.enableDualStack {
		return l4.provideDualStackHealthChecks(nodeNames, result)
	}
	return l4.provideIPv4HealthChecks(nodeNames, result)
}

func (l4 *L4) provideDualStackHealthChecks(nodeNames []string, result *L4ILBSyncResult) string {
	sharedHC := !helpers.RequestsOnlyLocalTraffic(l4.Service)
	hcResult := l4.healthChecks.EnsureHealthCheckWithDualStackFirewalls(l4.Service, l4.namer, sharedHC, meta.Global, utils.ILB, nodeNames, utils.NeedsIPv4(l4.Service), utils.NeedsIPv6(l4.Service), l4.network, l4.svcLogger)
	if hcResult.Err != nil {
		result.GCEResourceInError = hcResult.GceResourceInError
		result.Error = hcResult.Err
		return ""
	}

	if hcResult.HCFirewallRuleName != "" {
		result.Annotations[annotations.FirewallRuleForHealthcheckKey] = hcResult.HCFirewallRuleName
	}
	if hcResult.HCFirewallRuleIPv6Name != "" {
		result.Annotations[annotations.FirewallRuleForHealthcheckIPv6Key] = hcResult.HCFirewallRuleIPv6Name
	}
	result.Annotations[annotations.HealthcheckKey] = hcResult.HCName
	return hcResult.HCLink
}

func (l4 *L4) provideIPv4HealthChecks(nodeNames []string, result *L4ILBSyncResult) string {
	sharedHC := !helpers.RequestsOnlyLocalTraffic(l4.Service)
	hcResult := l4.healthChecks.EnsureHealthCheckWithFirewall(l4.Service, l4.namer, sharedHC, meta.Global, utils.ILB, nodeNames, l4.network, l4.svcLogger)
	result.ResourceUpdates.SetHealthCheck(hcResult.WasUpdated)
	result.ResourceUpdates.SetFirewallForHealthCheck(hcResult.WasFirewallUpdated)
	if hcResult.Err != nil {
		result.GCEResourceInError = hcResult.GceResourceInError
		result.Error = hcResult.Err
		return ""
	}
	result.Annotations[annotations.HealthcheckKey] = hcResult.HCName
	result.Annotations[annotations.FirewallRuleForHealthcheckKey] = hcResult.HCFirewallRuleName
	return hcResult.HCLink
}

func (l4 *L4) ensureDualStackResources(result *L4ILBSyncResult, nodeNames []string, options gce.ILBOptions, bs *composite.BackendService, existingIPv4FwdRule, existingIPv6FwdRule *composite.ForwardingRule, subnetworkURL, ipv4AddressToUse, ipv6AddressToUse string) {
	if utils.NeedsIPv4(l4.Service) {
		l4.ensureIPv4Resources(result, nodeNames, options, bs, existingIPv4FwdRule, subnetworkURL, ipv4AddressToUse)
	} else {
		l4.deleteIPv4ResourcesOnSync(result)
	}
	if utils.NeedsIPv6(l4.Service) {
		l4.ensureIPv6Resources(result, nodeNames, options, bs.SelfLink, existingIPv6FwdRule, ipv6AddressToUse)
	} else {
		l4.deleteIPv6ResourcesOnSync(result)
	}
}

// ensureIPv4Resources creates resources specific to IPv4 L4 Load Balancers:
// - IPv4 Forwarding Rule
// - IPv4 Firewall
func (l4 *L4) ensureIPv4Resources(result *L4ILBSyncResult, nodeNames []string, options gce.ILBOptions, bs *composite.BackendService, existingFR *composite.ForwardingRule, subnetworkURL, ipToUse string) {
	fr, fwdRuleSyncStatus, err := l4.ensureIPv4ForwardingRule(bs.SelfLink, options, existingFR, subnetworkURL, ipToUse)
	result.ResourceUpdates.SetForwardingRule(fwdRuleSyncStatus)
	if err != nil {
		l4.svcLogger.Error(err, "ensureIPv4Resources: Failed to ensure forwarding rule for L4 ILB Service")
		result.GCEResourceInError = annotations.ForwardingRuleResource
		result.Error = err
		return
	}

	switch fr.IPProtocol {
	case forwardingrules.ProtocolTCP:
		result.Annotations[annotations.TCPForwardingRuleKey] = fr.Name
	case forwardingrules.ProtocolUDP:
		result.Annotations[annotations.UDPForwardingRuleKey] = fr.Name
	case forwardingrules.ProtocolL3:
		result.Annotations[annotations.L3ForwardingRuleKey] = fr.Name
	}

	l4.ensureIPv4NodesFirewall(nodeNames, fr.IPAddress, result)
	if result.Error != nil {
		l4.svcLogger.Error(err, "ensureIPv4Resources: Failed to ensure nodes firewall for L4 ILB Service")
		return
	}

	result.Status = utils.AddIPToLBStatus(result.Status, fr.IPAddress)
}

func (l4 *L4) ensureIPv4NodesFirewall(nodeNames []string, ipAddress string, result *L4ILBSyncResult) {
	// DisableL4LBFirewall flag disables L4 FW enforcment to remove conflicts with firewall policies
	if l4.disableNodesFirewallProvisioning {
		l4.svcLogger.Info("Skipped ensuring IPv4 nodes firewall for L4 ILB Service to enable compatibility with firewall policies. " +
			"Be sure this cluster has a manually created global firewall policy in place.")
		return
	}
	start := time.Now()

	firewallName := l4.namer.L4Firewall(l4.Service.Namespace, l4.Service.Name)
	servicePorts := l4.Service.Spec.Ports
	protocol := utils.GetProtocol(servicePorts)
	portRanges := utils.GetServicePortRanges(servicePorts)
	allowed := []*compute.FirewallAllowed{
		{
			IPProtocol: string(protocol),
			Ports:      portRanges,
		},
	}

	if l4.enableMixedProtocol {
		allowed = firewalls.AllowedForService(servicePorts)
	}

	fwLogger := l4.svcLogger.WithValues("firewallName", firewallName)
	fwLogger.V(2).Info("Ensuring IPv4 nodes firewall for L4 ILB Service", "ipAddress", ipAddress, "protocol", protocol, "len(nodeNames)", len(nodeNames), "portRanges", portRanges)
	defer func() {
		fwLogger.V(2).Info("Finished ensuring IPv4 nodes firewall for L4 ILB Service", "timeTaken", time.Since(start))
	}()

	// ensure firewalls
	ipv4SourceRanges, err := utils.IPv4ServiceSourceRanges(l4.Service)
	if err != nil {
		result.Error = err
		return
	}
	// Add firewall rule for ILB traffic to nodes
	nodesFWRParams := firewalls.FirewallParams{
		Allowed:           allowed,
		SourceRanges:      ipv4SourceRanges,
		DestinationRanges: []string{ipAddress},
		Name:              firewallName,
		NodeNames:         nodeNames,
		L4Type:            utils.ILB,
		Network:           l4.network,
	}

	fwSyncStatus, err := firewalls.EnsureL4LBFirewallForNodes(l4.Service, &nodesFWRParams, l4.cloud, l4.recorder, fwLogger)
	result.ResourceUpdates.SetFirewallForNodes(fwSyncStatus)
	if err != nil {
		result.GCEResourceInError = annotations.FirewallRuleResource
		result.Error = err
		return
	}
	result.Annotations[annotations.FirewallRuleKey] = firewallName
}

func (l4 *L4) getServiceSubnetworkURL(options gce.ILBOptions) (string, error) {
	// Custom subnet feature is always enabled when running L4 controller.
	// Changes to subnet annotation will be picked up and reflected in the forwarding rule.
	// Removing the annotation will set the forwarding rule to use the default subnet.
	if options.SubnetName != "" {
		return l4.getSubnetworkURLByName(options.SubnetName)
	}
	return l4.network.SubnetworkURL, nil
}

func (l4 *L4) getSubnetworkURLByName(subnetName string) (string, error) {
	subnetwork, err := l4.cloud.GetSubnetwork(l4.cloud.Region(), subnetName)
	if err != nil {
		if utils.IsNotFoundError(err) {
			return "", utils.NewInvalidSubnetConfigurationError(l4.cloud.ProjectID(), subnetName)
		}
		return "", err
	}

	return subnetwork.SelfLink, nil
}

func (l4 *L4) hasAnnotation(annotationKey string) bool {
	if _, ok := l4.Service.Annotations[annotationKey]; ok {
		return true
	}
	return false
}

// getOldIPv4ForwardingRule returns old IPv4 forwarding rule, with checking backend service protocol, if it exists.
// This is useful when switching protocols of the service,
// because forwarding rule name depends on the protocol, and we need to get forwarding rule from the old protocol name.
func (l4 *L4) getOldIPv4ForwardingRule(existingBS *composite.BackendService) (*composite.ForwardingRule, error) {
	servicePorts := l4.Service.Spec.Ports
	bsProtocol := string(utils.GetProtocol(servicePorts))

	if l4.enableMixedProtocol {
		bsProtocol = backends.GetProtocol(servicePorts)
	}

	oldFRName := l4.GetFRName()
	if existingBS != nil && existingBS.Protocol != bsProtocol {
		fwdRuleProtocol := existingBS.Protocol
		if existingBS.Protocol == backends.ProtocolL3 {
			fwdRuleProtocol = forwardingrules.ProtocolL3
		}
		oldFRName = l4.getFRNameWithProtocol(fwdRuleProtocol)
	}

	return l4.forwardingRules.Get(oldFRName)
}

// determineBackendServiceLocalityPolicy returns the locality policy to be used for the backend service of the internal load balancer.
func (l4 *L4) determineBackendServiceLocalityPolicy() backends.LocalityLBPolicyType {
	// If the service has weighted load balancing enabled, the locality policy will be WEIGHTED_MAGLEV.
	if l4.enableWeightedLB {
		if annotations.HasWeightedLBPodsPerNodeAnnotation(l4.Service) {
			if l4.Service.Spec.ExternalTrafficPolicy == corev1.ServiceExternalTrafficPolicyTypeLocal {
				// If the service has the annotation "networking.gke.io/weighted-load-balancing = pods-per-node"
				// and the external traffic policy is local, weighted load balancing is enabled and the backend
				// service locality policy is set to WEIGHTED_MAGLEV.
				return backends.LocalityLBPolicyWeightedMaglev
			} else {
				// If the service has the annotation "networking.gke.io/weighted-load-balancing = pods-per-node"
				// and the external traffic policy is cluster, weighted load balancing is not enabled.
				l4.recorder.Eventf(l4.Service, corev1.EventTypeWarning, "UnsupportedConfiguration",
					"Weighted load balancing by pods-per-node has no effect with External Traffic Policy: Cluster.")
				// TODO(FelipeYepez) use LocalityLBPolicyMaglev once it does not require allow lisiting
				return backends.LocalityLBPolicyDefault
			}
		}
	}
	// The default unset locality lb policy is used to disable ILB Weighted Load Balancing
	return backends.LocalityLBPolicyDefault
}

func (l4 *L4) isWeightedLBPodsPerNode() bool {
	return backends.LocalityLBPolicyWeightedMaglev == l4.determineBackendServiceLocalityPolicy()
}

func (l4 *L4) isLBWithZonalAffinity() bool {
	return l4.enableZonalAffinity && l4.requireZonalAffinity(l4.Service)
}
