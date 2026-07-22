package experiments

import (
	"context"

	"k8s.io/klog/v2"
)

type MockClient struct {
	values            map[string]bool
	logger            klog.Logger
	boolFlagValueHook func(ctx context.Context, flagName string, values map[string]bool) (bool, error)
}

func NewMockClient(logger klog.Logger) *MockClient {
	return &MockClient{
		values: make(map[string]bool),
		logger: logger,
	}
}

func (m *MockClient) Set(flagName string, value bool) {
	m.values[flagName] = value
	m.logger.V(5).Info("set", "experiment", flagName, "value", value)
}

func (m *MockClient) Clear(flagNames ...string) {
	for _, f := range flagNames {
		delete(m.values, f)
		m.logger.V(5).Info("cleared", "experiment", f)
	}
}

func (m *MockClient) BoolFlagValueOrFailsafe(ctx context.Context, flagName string, failSafeValue bool) bool {
	if v, ok := m.values[flagName]; ok {
		m.logger.V(5).Info("found value", "experiment", flagName, "value", v)
		return v
	}
	m.logger.V(5).Info("falling back to fail safe", "experiment", flagName, "value", failSafeValue)
	return failSafeValue
}
