package experiments_test

import (
	"testing"
	"testing/synctest"
	"time"

	"k8s.io/ingress-gce/pkg/experiments"
	"k8s.io/klog/v2"
)

func TestCacheClientBoolCaching(t *testing.T) {
	// Time doesn't fly in synctest unless we tell it to using time.Sleep().
	synctest.Test(t, func(t *testing.T) {
		log := klog.Background()
		mockClient := experiments.NewMockClient(log)
		const experimentName = "TestSpace::EnableCache"
		mockClient.Set(experimentName, false)

		// Fill the cache
		ctx := t.Context()
		cacheClient := experiments.NewCacheClient(mockClient, log, 1*time.Hour)
		got := cacheClient.BoolFlagValueOrFailsafe(ctx, experimentName, true)
		if got != false {
			t.Fatalf("expected false, got %v, meaning failsafe value was used", got)
		}

		// Use the cache ignoring value updated
		mockClient.Set(experimentName, true)
		got = cacheClient.BoolFlagValueOrFailsafe(ctx, experimentName, true)
		if got != false {
			t.Fatalf("expected false, got %v, meaning failsafe value or fetch was used", got)
		}

		// After TTL fetch the value from the underlying client
		time.Sleep(2 * time.Hour)
		got = cacheClient.BoolFlagValueOrFailsafe(ctx, experimentName, true)
		if got != true {
			t.Fatalf("expected true, got %v, meaning failsafe value or fetch was used", got)
		}
	})
}

func TestCacheClientBoolFailsafes(t *testing.T) {
	ttl := 0 * time.Second
	log := klog.Background()
	mockClient := experiments.NewMockClient(log)
	cacheClient := experiments.NewCacheClient(mockClient, log, ttl)

	ctx := t.Context()
	got := cacheClient.BoolFlagValueOrFailsafe(ctx, "TestSpace::A", true)
	if got != true {
		t.Errorf("expected true, got %v, meaning failsafe value was used", got)
	}
	got = cacheClient.BoolFlagValueOrFailsafe(ctx, "TestSpace::B", false)
	if got != false {
		t.Errorf("expected false, got %v, meaning failsafe value was used", got)
	}
}
