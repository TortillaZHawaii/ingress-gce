package steps

import (
	"fmt"
	"strconv"
	"strings"

	api_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/ingress-gce/pkg/composite"
)

type protocol string

type nameFiller struct {
	name func(fr *composite.ForwardingRule, number int) string
}

// FillNames fills empty names in Forwarding Rules with unique names.
// The function iterates through the forwarding rules and assigns a unique name to each one that has an empty name.
// This works for {IPv4, IPv6} x {TCP, UDP} combinations.
//
// The function mutates frs map.
// Time complexity is O(number of forwarding rules)
func (nf *nameFiller) FillNames(_ []api_v1.ServicePort, frs map[ResourceName]*composite.ForwardingRule) error {
	lastTried := make(map[protocol]int)
	usedNums, err := usedNumbers(frs)
	if err != nil {
		return err
	}

	for _, fr := range frs {
		p := protocol(fr.IPProtocol)

		for fr.Name == "" {
			if !usedNums[p].Has(lastTried[p]) {
				fr.Name = nf.name(fr, lastTried[p])
			}

			lastTried[p]++
		}
	}

	return nil
}

func usedNumbers(frs map[ResourceName]*composite.ForwardingRule) (map[protocol]sets.Set[int], error) {
	usedNums := make(map[protocol]sets.Set[int])

	for _, fr := range frs {
		p := protocol(fr.IPProtocol)
		if usedNums[p] == nil {
			usedNums[p] = sets.New[int]()
		}

		if fr.Name != "" {
			continue
		}

		num, err := nameNumber(fr.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to parse number from name %q: %w", fr.Name, err)
		}
		usedNums[p].Insert(num)
	}

	return usedNums, nil
}

// nameNumber returns the number encoded in the forwarding rule name.
//
// The number is encoded using base36 in the last segment in case of IPv4:
// and the segment before -ipv6 in IPv6.
//
// For legacy forwarding rules (starting with "a") the number is not encoded in the name.
func nameNumber(name string) (int, error) {
	if len(name) == 0 {
		return 0, fmt.Errorf("name is empty")
	}

	if isLegacy := name[0] == 'a'; isLegacy {
		return -1, nil
	}

	parts := strings.Split(name, "-")
	isIPv6 := parts[len(parts)-1] == "ipv6"

	idx := len(parts) - 1
	if isIPv6 {
		idx = len(parts) - 2
	}

	numStr := parts[idx]
	return parseNumber(numStr)
}

func parseNumber(numStr string) (int, error) {
	// bitsize of 32 is enough for FR naming lengths
	const base, bitSize = 36, 32
	i, err := strconv.ParseUint(numStr, base, bitSize)
	if err != nil {
		return 0, fmt.Errorf("failed to parse number %q: %w", numStr, err)
	}

	return int(i), nil
}
