package forwardingrules

import (
	"slices"

	api_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/ingress-gce/pkg/composite"
)

func (cps *CleanPortsStrategy) Add(forwardingRules []*composite.ForwardingRule, service *api_v1.Service) []*composite.ForwardingRule {
	want := portsMap(service.Spec.Ports)
	have := haveMap(forwardingRules)
	diff := make(protocolPorts)

	for protocol := range want {
		diff[protocol] = want[protocol].Difference(have[protocol])
	}

	result := fillInRules(forwardingRules, diff)

	return result
}

func haveMap(forwardingRules []*composite.ForwardingRule) protocolPorts {
	m := make(protocolPorts)
	for _, fr := range forwardingRules {
		protocol := api_v1.Protocol(fr.IPProtocol)
		if _, ok := m[protocol]; !ok {
			m[protocol] = sets.New[string]()
		}

		m[protocol].Insert(fr.Ports...)
	}
	return m
}

func fillInRules(forwardingRules []*composite.ForwardingRule, diff protocolPorts) []*composite.ForwardingRule {
	protocolFRs := splitByProtocol(forwardingRules)
	allRules := make([]*composite.ForwardingRule, 0, len(forwardingRules))
	for protocol, frs := range protocolFRs {
		portsToAdd := diff[protocol]
		if portsToAdd.Len() == 0 {
			continue
		}

		frs, portsLeft := addPortsToExistingRules(frs, portsToAdd)
		frs = append(frs, createNewRulesForLeftPorts(portsLeft, protocol)...)
		allRules = append(allRules, frs...)
	}

	return allRules
}

func addPortsToExistingRules(forwardingRules []*composite.ForwardingRule, portsToAdd sets.Set[string]) ([]*composite.ForwardingRule, sets.Set[string]) {
	// ascending by ports used
	// descending by free slots
	// to minimize number of modify operations
	slices.SortFunc(forwardingRules, func(a, b *composite.ForwardingRule) int {
		return len(a.Ports) - len(b.Ports)
	})

	for _, fr := range forwardingRules {
		reachedFullFRs := MaxDiscretePorts == len(fr.Ports)
		if portsToAdd.Len() == 0 || reachedFullFRs {
			break
		}

		for len(fr.Ports) < MaxDiscretePorts && portsToAdd.Len() > 0 {
			port, ok := portsToAdd.PopAny()
			if !ok { // This shouldn't happen as we check for Len before popping
				break
			}
			fr.Ports = append(fr.Ports, port)
		}
	}

	return forwardingRules, portsToAdd
}

func createNewRulesForLeftPorts(portsLeft sets.Set[string], protocol api_v1.Protocol) []*composite.ForwardingRule {
	frsNeededCount := (portsLeft.Len() + MaxDiscretePorts - 1) / MaxDiscretePorts
	frs := make([]*composite.ForwardingRule, 0, frsNeededCount)

	for _ = range frsNeededCount {
		ports := make([]string, 0, MaxDiscretePorts)
		for len(ports) < MaxDiscretePorts && portsLeft.Len() > 0 {
			port, ok := portsLeft.PopAny()
			if !ok { // This shouldn't happen as we check for Len before popping
				break
			}
			ports = append(ports, port)
		}

		frs = append(frs, &composite.ForwardingRule{
			// Other fields will be filled in at a later stage in a pipeline
			Ports:      ports,
			IPProtocol: string(protocol),
		})
	}

	return frs
}

func splitByProtocol(forwardingRules []*composite.ForwardingRule) map[api_v1.Protocol][]*composite.ForwardingRule {
	m := make(map[api_v1.Protocol][]*composite.ForwardingRule)
	for _, fr := range forwardingRules {
		p := api_v1.Protocol(fr.IPProtocol)
		m[p] = append(m[p], fr)
	}
	return m
}
