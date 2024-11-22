package forwardingrules

import (
	"errors"
	"fmt"
	"strings"
	"time"

	api_v1 "k8s.io/api/core/v1"
	"k8s.io/ingress-gce/pkg/composite"
	"k8s.io/ingress-gce/pkg/events"
	"k8s.io/ingress-gce/pkg/utils"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud"
	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"
)

const (
	// maxForwardedPorts is the maximum number of ports that can be specified in an Forwarding Rule
	maxForwardedPorts = 5
	// addressAlreadyInUseMessageExternal is the error message string returned by the compute API
	// when creating an external forwarding rule that uses a conflicting IP address.
	addressAlreadyInUseMessageExternal = "Specified IP address is in-use and would result in a conflict."
)

type Namer interface {
	L4ForwardingRule(namespace, name, protocol string) string
}

type ManagerELB struct {
	Namer    Namer
	Provider *ForwardingRules
	Recorder record.EventRecorder
	Logger   logr.Logger

	Service *api_v1.Service
}

type EnsureELBConfig struct {
	BackendServiceLink string
	IP                 string
}

type EnsureELBResult struct {
	UDPFwdRule *composite.ForwardingRule
	TCPFwdRule *composite.ForwardingRule
	IPManaged  bool
	SyncStatus utils.ResourceSyncStatus
}

func (m *ManagerELB) EnsureIPv4(cfg *EnsureELBConfig) (*EnsureELBResult, error) {
	var tcpErr, udpErr error
	var tcpSync, udpSync utils.ResourceSyncStatus

	svcPorts := m.Service.Spec.Ports
	res := &EnsureELBResult{
		SyncStatus: utils.ResourceResync,
	}

	needsTCP, needsUDP := NeedsTCP(svcPorts), NeedsUDP(svcPorts)
	needsMixed := needsTCP && needsUDP

	legacy, err := m.getLegacy()
	if err != nil {
		return res, err
	}

	if legacy != nil && !needsMixed {
		// TODO: handle legacy
	}

	if needsTCP {
		res.TCPFwdRule, tcpSync, tcpErr = m.ensure(cfg, m.name("TCP"), "TCP")
	} else {
		tcpErr = m.delete("TCP")
	}

	if needsUDP {
		res.UDPFwdRule, udpSync, udpErr = m.ensure(cfg, m.name("UDP"), "UDP")
	} else {
		udpErr = m.delete("UDP")
	}

	res.SyncStatus = tcpSync || udpSync
	return res, errors.Join(tcpErr, udpErr)
}

// ensure has similar implementation to the L4NetLB.ensureIPv4ForwardingRule,
// but can use multiple names for fwd rule.
// This will:
// * compare existing rule to wanted
// * if doesnt exist 	-> create
// * if equal 			-> do nothing
// * if can be patched 	-> patch
// * else 				-> delete and recreate
func (m *ManagerELB) ensure(cfg *EnsureELBConfig, name, protocol string) (*composite.ForwardingRule, utils.ResourceSyncStatus, error) {
	start := time.Now()
	log := m.Logger.
		WithValues("forwardingRuleName", name).
		WithValues("protocol", protocol).V(2)
	log.Info("Ensuring external forwarding rule for L4 NetLB Service", "backendServiceLink", cfg.BackendServiceLink)
	defer func() {
		log.Info("Finished ensuring external forwarding rule for L4 NetLB Service", "timeTaken", time.Since(start))
	}()

	existing, err := m.Provider.Get(name)
	if err != nil {
		log.Error(err, "Provider.Get returned error")
		return nil, utils.ResourceResync, err
	}

	wanted, err := m.buildWanted(cfg, name, protocol)
	if err != nil {
		log.Error(err, "buildWanted returned error")
		return nil, utils.ResourceResync, err
	}

	// Exists
	if existing == nil {
		if err := m.Provider.Create(wanted); err != nil {
			log.Error(err, "Provider.Create returned error")
			return nil, utils.ResourceUpdate, err
		}
		return m.getAfterUpdate(name)
	}

	// Can't update
	if networkMismatch := existing.NetworkTier != wanted.NetworkTier; networkMismatch {
		resource := fmt.Sprintf("Forwarding rule (%v)", name)
		networkTierMismatchErr := utils.NewNetworkTierErr(resource, wanted.NetworkTier, wanted.NetworkTier)
		return nil, utils.ResourceUpdate, networkTierMismatchErr
	}

	// Equal
	if equal, err := EqualIPv4(existing, wanted); err != nil {
		log.Error(err, "EqualIPV4 returned error")
		return nil, utils.ResourceResync, err
	} else if equal {
		return existing, utils.ResourceResync, err
	}

	// Patchable
	if patchable, filtered := PatchableIPv4(existing, wanted); patchable {
		if err := m.Provider.Patch(filtered); err != nil {
			return nil, utils.ResourceUpdate, err
		}
		return m.getAfterUpdate(name)
	}

	// Recreate
	if err := m.recreate(wanted); err != nil {
		return nil, utils.ResourceResync, err
	}
	return m.getAfterUpdate(name)
}

func (m *ManagerELB) buildWanted(cfg *EnsureELBConfig, name, protocol string) (*composite.ForwardingRule, error) {
	const version = meta.VersionGA
	const scheme = string(cloud.SchemeExternal)
	protocol = strings.ToUpper(protocol)
	if protocol != "TCP" && protocol != "UDP" {
		return nil, fmt.Errorf("Unknown protocol %s, expected TCP or UDP", protocol)
	}

	svcKey := utils.ServiceKeyFunc(m.Service.Namespace, m.Service.Name)
	desc, err := utils.MakeL4LBServiceDescription(svcKey, cfg.IP, version, false, utils.XLB)
	if err != nil {
		return nil, fmt.Errorf("Failed to compute description for forwarding rule %s, err: %w", name, err)
	}

	ports := GetPorts(m.Service.Spec.Ports, api_v1.Protocol(protocol))
	var portRange string
	if len(ports) > maxForwardedPorts {
		portRange = utils.MinMaxPortRange(ports)
		ports = nil
	}

	netTier, _ := utils.GetNetworkTier(m.Service)

	return &composite.ForwardingRule{
		Name:                name,
		Description:         desc,
		IPAddress:           cfg.IP,
		IPProtocol:          protocol,
		Ports:               ports,
		PortRange:           portRange,
		LoadBalancingScheme: scheme,
		BackendService:      cfg.BackendServiceLink,
		NetworkTier:         netTier.ToGCEValue(),
	}, nil
}

// Provider should return nil, nil if the rule doesn't exist
func (m *ManagerELB) getLegacy() (*composite.ForwardingRule, error) {
	name := utils.LegacyForwardingRuleName(m.Service)
	return m.Provider.Get(name)
}

func (m *ManagerELB) getAfterUpdate(name string) (*composite.ForwardingRule, utils.ResourceSyncStatus, error) {
	found, err := m.Provider.Get(name)
	if err != nil {
		return nil, utils.ResourceUpdate, err
	}
	if found == nil {
		return nil, utils.ResourceUpdate, fmt.Errorf("Forwarding rule %s not found", name)
	}

	return found, utils.ResourceUpdate, nil
}

func (m *ManagerELB) DeleteIPv4() error {
	tcpErr := m.delete("tcp")
	udpErr := m.delete("udp")
	legacyErr := m.deleteLegacy()

	return errors.Join(tcpErr, udpErr, legacyErr)
}

func (m *ManagerELB) deleteLegacy() error {
	name := utils.LegacyForwardingRuleName(m.Service)
	err := m.Provider.Delete(name)
	return utils.IgnoreHTTPNotFound(err)
}

func (m *ManagerELB) delete(protocol string) error {
	name := m.name(protocol)
	err := m.Provider.Delete(name)
	return utils.IgnoreHTTPNotFound(err)
}

func (m *ManagerELB) recreate(wanted *composite.ForwardingRule) error {
	if err := m.Provider.Delete(wanted.Name); err != nil {
		return err
	}

	if err := m.Provider.Create(wanted); err != nil {
		return err
	}

	return nil
}

func (m *ManagerELB) name(protocol string) string {
	return m.Namer.L4ForwardingRule(
		m.Service.Namespace, m.Service.Name, strings.ToLower(protocol),
	)
}

func (m *ManagerELB) recordf(messageFmt string, args ...any) {
	m.Recorder.Eventf(m.Service, api_v1.EventTypeNormal, events.SyncIngress, messageFmt, args)
}
