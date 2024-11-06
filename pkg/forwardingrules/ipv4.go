package forwardingrules

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cloud-provider-gcp/providers/gce"
	"k8s.io/ingress-gce/pkg/annotations"
	"k8s.io/ingress-gce/pkg/composite"
	"k8s.io/ingress-gce/pkg/events"
	"k8s.io/ingress-gce/pkg/flags"
	"k8s.io/ingress-gce/pkg/utils"
	"k8s.io/klog/v2"
)

const (
	// maxForwardedPorts is the maximum number of ports that can be specified in an Forwarding Rule
	maxForwardedPorts = 5
	// addressAlreadyInUseMessageExternal is the error message string returned by the compute API
	// when creating an external forwarding rule that uses a conflicting IP address.
	addressAlreadyInUseMessageExternal = "Specified IP address is in-use and would result in a conflict."
	// addressAlreadyInUseMessageInternal is the error message string returned by the compute API
	// when creating an internal forwarding rule that uses a conflicting IP address.
	addressAlreadyInUseMessageInternal = "IP_IN_USE_BY_ANOTHER_RESOURCE"
)

// ensureIPv4ForwardingRule creates a forwarding rule with the given name, if it does not exist. It updates the existing
// forwarding rule if needed.
func (l4 *L4) ensureIPv4ForwardingRule(bsLink string, options gce.ILBOptions, existingFwdRule *composite.ForwardingRule, subnetworkURL, ipToUse string) (*composite.ForwardingRule, utils.ResourceSyncStatus, error) {
	start := time.Now()

	// version used for creating the existing forwarding rule.
	version := meta.VersionGA
	frName := l4.GetFRName()

	frLogger := l4.svcLogger.WithValues("forwardingRuleName", frName)
	frLogger.V(2).Info("Ensuring internal forwarding rule for L4 ILB Service", "backendServiceLink", bsLink)
	defer func() {
		frLogger.V(2).Info("Finished ensuring internal forwarding rule for L4 ILB Service", "timeTaken", time.Since(start))
	}()

	servicePorts := l4.Service.Spec.Ports
	ports := utils.GetPorts(servicePorts)
	protocol := utils.GetProtocol(servicePorts)
	// Create the forwarding rule
	frDesc, err := utils.MakeL4LBServiceDescription(utils.ServiceKeyFunc(l4.Service.Namespace, l4.Service.Name), ipToUse,
		version, false, utils.ILB)
	if err != nil {
		return nil, utils.ResourceResync, fmt.Errorf("Failed to compute description for forwarding rule %s, err: %w", frName,
			err)
	}

	newFwdRule := &composite.ForwardingRule{
		Name:                frName,
		IPAddress:           ipToUse,
		Ports:               ports,
		IPProtocol:          string(protocol),
		LoadBalancingScheme: string(cloud.SchemeInternal),
		Subnetwork:          subnetworkURL,
		Network:             l4.network.NetworkURL,
		NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
		Version:             version,
		BackendService:      bsLink,
		AllowGlobalAccess:   options.AllowGlobalAccess,
		Description:         frDesc,
	}
	if len(ports) > maxForwardedPorts {
		newFwdRule.Ports = nil
		newFwdRule.AllPorts = true
	}

	if existingFwdRule != nil {
		equal, err := Equal(existingFwdRule, newFwdRule)
		if err != nil {
			return nil, utils.ResourceResync, err
		}
		if equal {
			// nothing to do
			frLogger.V(2).Info("ensureIPv4ForwardingRule: Skipping update of unchanged forwarding rule")
			return existingFwdRule, utils.ResourceResync, nil
		}
		frDiff := cmp.Diff(existingFwdRule, newFwdRule)
		frLogger.V(2).Info("ensureIPv4ForwardingRule: forwarding rule changed.",
			"existingForwardingRule", fmt.Sprintf("%+v", existingFwdRule), "newForwardingRule", fmt.Sprintf("%+v", newFwdRule), "diff", frDiff)

		filtered, patchable := filterPatchableFields(existingFwdRule, newFwdRule)
		if patchable {
			if err = l4.forwardingRules.Patch(filtered); err != nil {
				return nil, utils.ResourceUpdate, err
			}
			l4.recorder.Eventf(l4.Service, corev1.EventTypeNormal, events.SyncIngress, "ForwardingRule %s patched", existingFwdRule.Name)
		} else {
			if err := l4.updateForwardingRule(existingFwdRule, newFwdRule, frLogger); err != nil {
				return nil, utils.ResourceUpdate, err
			}
		}
	} else {
		if err = l4.createFwdRule(newFwdRule, frLogger); err != nil {
			return nil, utils.ResourceUpdate, err
		}
		l4.recorder.Eventf(l4.Service, corev1.EventTypeNormal, events.SyncIngress, "ForwardingRule %s created", newFwdRule.Name)
	}

	readFwdRule, err := l4.forwardingRules.Get(newFwdRule.Name)
	if err != nil {
		return nil, utils.ResourceUpdate, err
	}
	if readFwdRule == nil {
		return nil, utils.ResourceUpdate, fmt.Errorf("Forwarding Rule %s not found", frName)
	}
	return readFwdRule, utils.ResourceUpdate, nil
}

func (l4 *L4) updateForwardingRule(existingFwdRule, newFr *composite.ForwardingRule, frLogger klog.Logger) error {
	if err := l4.forwardingRules.Delete(existingFwdRule.Name); err != nil {
		return err
	}
	l4.recorder.Eventf(l4.Service, corev1.EventTypeNormal, events.SyncIngress, "ForwardingRule %s deleted", existingFwdRule.Name)

	if err := l4.createFwdRule(newFr, frLogger); err != nil {
		return err
	}
	l4.recorder.Eventf(l4.Service, corev1.EventTypeNormal, events.SyncIngress, "ForwardingRule %s re-created", newFr.Name)
	return nil
}

func (l4 *L4) createFwdRule(newFr *composite.ForwardingRule, frLogger klog.Logger) error {
	frLogger.V(2).Info("ensureIPv4ForwardingRule: Creating/Recreating forwarding rule")
	if err := l4.forwardingRules.Create(newFr); err != nil {
		if isAddressAlreadyInUseError(err) {
			return utils.NewIPConfigurationError(newFr.IPAddress, err.Error())
		}
		return err
	}
	return nil
}

// ensureIPv4ForwardingRule creates a forwarding rule with the given name for L4NetLB,
// if it does not exist. It updates the existing forwarding rule if needed.
func (l4netlb *L4NetLB) ensureIPv4ForwardingRule(bsLink string) (*composite.ForwardingRule, IPAddressType, utils.ResourceSyncStatus, error) {
	frName := l4netlb.frName()

	start := time.Now()
	frLogger := l4netlb.svcLogger.WithValues("forwardingRuleName", frName)
	frLogger.V(2).Info("Ensuring external forwarding rule for L4 NetLB Service", "backendServiceLink", bsLink)
	defer func() {
		frLogger.V(2).Info("Finished ensuring external forwarding rule for L4 NetLB Service", "timeTaken", time.Since(start))
	}()

	// version used for creating the existing forwarding rule.
	version := meta.VersionGA
	existingFwdRule, err := l4netlb.forwardingRules.Get(frName)
	if err != nil {
		frLogger.Error(err, "l4netlb.forwardingRules.Get returned error")
		return nil, IPAddrUndefined, utils.ResourceResync, err
	}

	// Determine IP which will be used for this LB. If no forwarding rule has been established
	// or specified in the Service spec, then requestedIP = "".
	ipToUse, err := ipv4AddrToUse(l4netlb.cloud, l4netlb.recorder, l4netlb.Service, existingFwdRule, "")
	if err != nil {
		frLogger.Error(err, "ipv4AddrToUse for service returned error")
		return nil, IPAddrUndefined, utils.ResourceResync, err
	}
	frLogger.V(2).Info("ensureIPv4ForwardingRule: Got LoadBalancer IP", "ip", ipToUse)

	netTier, isFromAnnotation := utils.GetNetworkTier(l4netlb.Service)
	var isIPManaged IPAddressType
	// If the network is not a legacy network, use the address manager
	if !l4netlb.cloud.IsLegacyNetwork() {
		nm := types.NamespacedName{Namespace: l4netlb.Service.Namespace, Name: l4netlb.Service.Name}.String()
		addrMgr := newAddressManager(l4netlb.cloud, nm, l4netlb.cloud.Region() /*subnetURL = */, "", frName, ipToUse, cloud.SchemeExternal, netTier, IPv4Version, frLogger)

		// If network tier annotation in Service Spec is present
		// check if it matches network tiers from forwarding rule and external ip Address.
		// If they do not match, tear down the existing resources with the wrong tier.
		if isFromAnnotation {
			if err := l4netlb.tearDownResourcesWithWrongNetworkTier(existingFwdRule, netTier, addrMgr, frLogger); err != nil {
				return nil, IPAddrUndefined, utils.ResourceResync, err
			}
		}

		ipToUse, isIPManaged, err = addrMgr.HoldAddress()
		if err != nil {
			return nil, IPAddrUndefined, utils.ResourceResync, err
		}
		frLogger.V(2).Info("ensureIPv4ForwardingRule: reserved IP for the forwarding rule", "ip", ipToUse)
		defer func() {
			// Release the address that was reserved, in all cases. If the forwarding rule was successfully created,
			// the ephemeral IP is not needed anymore. If it was not created, the address should be released to prevent leaks.
			if err := addrMgr.ReleaseAddress(); err != nil {
				frLogger.Error(err, "ensureIPv4ForwardingRule: failed to release address reservation, possibly causing an orphan")
			}
		}()
	}

	svcPorts := l4netlb.Service.Spec.Ports
	ports := utils.GetPorts(svcPorts)
	portRange := utils.MinMaxPortRange(svcPorts)
	protocol := utils.GetProtocol(svcPorts)
	serviceKey := utils.ServiceKeyFunc(l4netlb.Service.Namespace, l4netlb.Service.Name)
	frDesc, err := utils.MakeL4LBServiceDescription(serviceKey, ipToUse, version, false, utils.XLB)
	if err != nil {
		return nil, IPAddrUndefined, utils.ResourceResync, fmt.Errorf("Failed to compute description for forwarding rule %s, err: %w", frName,
			err)
	}
	newFwdRule := &composite.ForwardingRule{
		Name:                frName,
		Description:         frDesc,
		IPAddress:           ipToUse,
		IPProtocol:          string(protocol),
		PortRange:           portRange,
		LoadBalancingScheme: string(cloud.SchemeExternal),
		BackendService:      bsLink,
		NetworkTier:         netTier.ToGCEValue(),
	}
	if len(ports) <= maxForwardedPorts && flags.F.EnableDiscretePortForwarding {
		newFwdRule.Ports = ports
		newFwdRule.PortRange = ""
	}

	if existingFwdRule != nil {
		if existingFwdRule.NetworkTier != newFwdRule.NetworkTier {
			resource := fmt.Sprintf("Forwarding rule (%v)", frName)
			networkTierMismatchError := utils.NewNetworkTierErr(resource, existingFwdRule.NetworkTier, newFwdRule.NetworkTier)
			return nil, IPAddrUndefined, utils.ResourceUpdate, networkTierMismatchError
		}
		equal, err := Equal(existingFwdRule, newFwdRule)
		if err != nil {
			return existingFwdRule, IPAddrUndefined, utils.ResourceResync, err
		}
		if equal {
			// nothing to do
			frLogger.V(2).Info("ensureIPv4ForwardingRule: Skipping update of unchanged forwarding rule")
			return existingFwdRule, isIPManaged, utils.ResourceResync, nil
		}
		frDiff := cmp.Diff(existingFwdRule, newFwdRule)
		frLogger.V(2).Info("ensureIPv4ForwardingRule: forwarding rule changed.",
			"existingForwardingRule", fmt.Sprintf("%+v", existingFwdRule), "newForwardingRule", fmt.Sprintf("%+v", newFwdRule), "diff", frDiff)

		filtered, patchable := filterPatchableFields(existingFwdRule, newFwdRule)
		if patchable {
			if err = l4netlb.forwardingRules.Patch(filtered); err != nil {
				return nil, IPAddrUndefined, utils.ResourceUpdate, err
			}
			l4netlb.recorder.Eventf(l4netlb.Service, corev1.EventTypeNormal, events.SyncIngress, "ForwardingRule %s patched", existingFwdRule.Name)
		} else {
			if err := l4netlb.updateForwardingRule(existingFwdRule, newFwdRule, frLogger); err != nil {
				return nil, IPAddrUndefined, utils.ResourceUpdate, err
			}
		}

	} else {
		if err = l4netlb.createFwdRule(newFwdRule, frLogger); err != nil {
			return nil, IPAddrUndefined, utils.ResourceUpdate, err
		}
		l4netlb.recorder.Eventf(l4netlb.Service, corev1.EventTypeNormal, events.SyncIngress, "ForwardingRule %s created", newFwdRule.Name)
	}
	createdFr, err := l4netlb.forwardingRules.Get(newFwdRule.Name)
	if err != nil {
		return nil, IPAddrUndefined, utils.ResourceUpdate, err
	}
	if createdFr == nil {
		return nil, IPAddrUndefined, utils.ResourceUpdate, fmt.Errorf("forwarding rule %s not found", newFwdRule.Name)
	}
	return createdFr, isIPManaged, utils.ResourceUpdate, err
}

func (l4netlb *L4NetLB) updateForwardingRule(existingFwdRule, newFr *composite.ForwardingRule, frLogger klog.Logger) error {
	if err := l4netlb.forwardingRules.Delete(existingFwdRule.Name); err != nil {
		return err
	}
	l4netlb.recorder.Eventf(l4netlb.Service, corev1.EventTypeNormal, events.SyncIngress, "ForwardingRule %s deleted", existingFwdRule.Name)

	if err := l4netlb.createFwdRule(newFr, frLogger); err != nil {
		return err
	}
	l4netlb.recorder.Eventf(l4netlb.Service, corev1.EventTypeNormal, events.SyncIngress, "ForwardingRule %s re-created", newFr.Name)
	return nil
}

func (l4netlb *L4NetLB) createFwdRule(newFr *composite.ForwardingRule, frLogger klog.Logger) error {
	frLogger.V(2).Info("ensureIPv4ForwardingRule: Creating/Recreating forwarding rule")
	if err := l4netlb.forwardingRules.Create(newFr); err != nil {
		if isAddressAlreadyInUseError(err) {
			return utils.NewIPConfigurationError(newFr.IPAddress, addressAlreadyInUseMessageExternal)
		}
		return err
	}
	return nil
}

// tearDownResourcesWithWrongNetworkTier removes forwarding rule or IP address if its Network Tier differs from desired.
func (l4netlb *L4NetLB) tearDownResourcesWithWrongNetworkTier(existingFwdRule *composite.ForwardingRule, svcNetTier cloud.NetworkTier, am *addressManager, frLogger klog.Logger) error {
	if existingFwdRule != nil && existingFwdRule.NetworkTier != svcNetTier.ToGCEValue() {
		err := l4netlb.forwardingRules.Delete(existingFwdRule.Name)
		if err != nil {
			frLogger.Error(err, "l4netlb.forwardingRules.Delete returned error, want nil")
		}
	}
	return am.TearDownAddressIPIfNetworkTierMismatch()
}

func filterPatchableFields(existing, new *composite.ForwardingRule) (*composite.ForwardingRule, bool) {
	existingCopy := *existing
	newCopy := *new

	// Set AllowGlobalAccess and NetworkTier fields to the same value in both copies
	existingCopy.AllowGlobalAccess = new.AllowGlobalAccess
	existingCopy.NetworkTier = new.NetworkTier

	equal, err := Equal(&existingCopy, &newCopy)

	// Something is different other than AllowGlobalAccess and NetworkTier
	if err != nil || !equal {
		return nil, false
	}

	filtered := &composite.ForwardingRule{}
	filtered.Id = existing.Id
	filtered.Name = existing.Name
	// AllowGlobalAccess is in the ForceSendFields list, it always need to have the right value
	filtered.AllowGlobalAccess = new.AllowGlobalAccess
	// Send NetworkTier in the patch request only if it has been updated
	if existing.NetworkTier != new.NetworkTier {
		filtered.NetworkTier = new.NetworkTier
	}
	return filtered, true
}

// ipv4AddrToUse determines which IPv4 address needs to be used in the ForwardingRule,
// address evaluated in the following order:
//
//  1. Use static addresses annotation "networking.gke.io/load-balancer-ip-addresses".
//  2. Use .Spec.LoadBalancerIP (old field, was deprecated).
//  3. Use existing forwarding rule IP. If subnetwork was changed (or no existing IP),
//     reset the IP (by returning empty string).
func ipv4AddrToUse(cloud *gce.Cloud, recorder record.EventRecorder, svc *v1.Service, fwdRule *composite.ForwardingRule, requestedSubnet string) (string, error) {
	// Get value from new annotation which support both IPv4 and IPv6
	ipv4FromAnnotation, err := annotations.FromService(svc).IPv4AddressAnnotation(cloud)
	if err != nil {
		return "", err
	}
	if ipv4FromAnnotation != "" {
		if svc.Spec.LoadBalancerIP != "" {
			recorder.Event(svc, v1.EventTypeNormal, "MixedStaticIP", "Found both .Spec.LoadBalancerIP and \"networking.gke.io/load-balancer-ip-addresses\" annotation. Consider using annotation only.")
		}
		return ipv4FromAnnotation, nil
		// if no value from annotation (for example, annotation has only IPv6 addresses) -- continue
	}
	if svc.Spec.LoadBalancerIP != "" {
		return svc.Spec.LoadBalancerIP, nil
	}
	if fwdRule == nil {
		return "", nil
	}
	if requestedSubnet != fwdRule.Subnetwork {
		// reset ip address since subnet is being changed.
		return "", nil
	}
	return fwdRule.IPAddress, nil
}

func isAddressAlreadyInUseError(err error) bool {
	// Bad request HTTP status (400) is returned for external Forwarding Rules.
	alreadyInUseExternal := utils.IsHTTPErrorCode(err, http.StatusBadRequest) && strings.Contains(err.Error(), addressAlreadyInUseMessageExternal)
	// Conflict HTTP status (409) is returned for internal Forwarding Rules.
	alreadyInUseInternal := utils.IsHTTPErrorCode(err, http.StatusConflict) && strings.Contains(err.Error(), addressAlreadyInUseMessageInternal)
	return alreadyInUseExternal || alreadyInUseInternal
}
