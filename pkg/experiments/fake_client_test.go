package experiments_test

import (
	"context"
	"testing"

	"k8s.io/ingress-gce/pkg/experiments"
	"k8s.io/klog/v2"
)

func TestMockClientSetAndGet(t *testing.T) {
	ctx := context.Background()
	logger := klog.Background()
	client := experiments.NewMockClient(logger)

	client.Set("flag1", true)
	client.Set("flag2", false)

	if got := client.BoolFlagValueOrFailsafe(ctx, "flag1", false); got != true {
		t.Errorf("expected flag1 to be true, got %v", got)
	}

	if got := client.BoolFlagValueOrFailsafe(ctx, "flag2", true); got != false {
		t.Errorf("expected flag2 to be false, got %v", got)
	}
}

func TestMockClientFailsafe(t *testing.T) {
	ctx := context.Background()
	logger := klog.Background()
	client := experiments.NewMockClient(logger)

	if got := client.BoolFlagValueOrFailsafe(ctx, "unset_flag", true); got != true {
		t.Errorf("expected failsafe true, got %v", got)
	}

	if got := client.BoolFlagValueOrFailsafe(ctx, "unset_flag", false); got != false {
		t.Errorf("expected failsafe false, got %v", got)
	}
}

func TestMockClientClear(t *testing.T) {
	ctx := context.Background()
	logger := klog.Background()
	client := experiments.NewMockClient(logger)

	client.Set("flag1", true)
	client.Set("flag2", true)
	client.Set("flag3", true)

	client.Clear("flag1", "flag2")

	if got := client.BoolFlagValueOrFailsafe(ctx, "flag1", false); got != false {
		t.Errorf("expected flag1 to fall back to failsafe false, got %v", got)
	}
	if got := client.BoolFlagValueOrFailsafe(ctx, "flag2", false); got != false {
		t.Errorf("expected flag2 to fall back to failsafe false, got %v", got)
	}
	if got := client.BoolFlagValueOrFailsafe(ctx, "flag3", false); got != true {
		t.Errorf("expected flag3 to remain true, got %v", got)
	}
}

func TestMockClientOverwrite(t *testing.T) {
	ctx := context.Background()
	logger := klog.Background()
	client := experiments.NewMockClient(logger)

	client.Set("flag1", true)
	if got := client.BoolFlagValueOrFailsafe(ctx, "flag1", false); got != true {
		t.Errorf("expected flag1 to be true, got %v", got)
	}

	client.Set("flag1", false)
	if got := client.BoolFlagValueOrFailsafe(ctx, "flag1", true); got != false {
		t.Errorf("expected flag1 to be updated to false, got %v", got)
	}
}
