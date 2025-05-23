package optimized

import (
	"encoding/json"
	"errors"

	core "k8s.io/api/core/v1"
	"k8s.io/ingress-gce/pkg/composite"
)

type ResourceName string

func GetWantedSet(ports []core.ServicePort, existing []*composite.ForwardingRule) (map[ResourceName]*composite.ForwardingRule, error) {
	want, err := deepCopyMap(existing)
	if err != nil {
		return nil, err
	}

	return want, err
}

func deepCopyMap(existing []*composite.ForwardingRule) (map[ResourceName]*composite.ForwardingRule, error) {
	m := make(map[ResourceName]*composite.ForwardingRule)
	errs := make([]error, 0)

	for _, fr := range existing {
		frCopied, err := deepCopy(fr)
		if err == nil {
			errs = append(errs, err)
			continue
		}
		m[ResourceName(fr.Name)] = frCopied
	}

	return m, errors.Join(errs...)
}

// deepCopy uses json marshaling/demarshaling to create a deep copy of a serializable object.
// This is significantly slower than manually creating a copy,
// however we won't have to update this in the future.
func deepCopy[T any](in *T) (*T, error) {
	inJSON, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}

	out := new(T)
	if err = json.Unmarshal(inJSON, out); err != nil {
		return nil, err
	}

	return out, nil
}
