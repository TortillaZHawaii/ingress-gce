package experiments_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/ingress-gce/pkg/experiments"
	expgrpc "k8s.io/ingress-gce/pkg/experiments/grpc"
	"k8s.io/klog/v2"
)

func TestGRPCClientSuccess(t *testing.T) {
	ctx := context.Background()
	logger := klog.Background()

	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "exp.sock")

	mockClient := experiments.NewMockClient(logger)
	mockClient.Set("FeatureA", true)
	mockClient.Set("FeatureB", false)

	server := expgrpc.NewServer(mockClient, logger)
	_, err := server.StartOnUnixSocket(socketPath)
	if err != nil {
		t.Fatalf("failed to start gRPC server: %v", err)
	}
	defer server.Stop()

	client, err := experiments.NewGRPCClient(socketPath, logger)
	if err != nil {
		t.Fatalf("failed to create gRPC client: %v", err)
	}
	defer client.Close()

	if got := client.BoolFlagValueOrFailsafe(ctx, "FeatureA", false); got != true {
		t.Errorf("expected FeatureA to be true, got %v", got)
	}

	if got := client.BoolFlagValueOrFailsafe(ctx, "FeatureB", true); got != false {
		t.Errorf("expected FeatureB to be false, got %v", got)
	}

	if got := client.BoolFlagValueOrFailsafe(ctx, "FeatureUnset", true); got != true {
		t.Errorf("expected FeatureUnset to return failsafe true, got %v", got)
	}

	if got := client.BoolFlagValueOrFailsafe(ctx, "FeatureUnset", false); got != false {
		t.Errorf("expected FeatureUnset to return failsafe false, got %v", got)
	}
}

func TestGRPCClientFallbackWhenServerUnreachable(t *testing.T) {
	ctx := context.Background()
	logger := klog.Background()

	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "non_existent.sock")

	client, err := experiments.NewGRPCClientWithTimeout(socketPath, logger, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to create gRPC client: %v", err)
	}
	defer client.Close()

	if got := client.BoolFlagValueOrFailsafe(ctx, "FeatureA", true); got != true {
		t.Errorf("expected failsafe true when server unreachable, got %v", got)
	}

	if got := client.BoolFlagValueOrFailsafe(ctx, "FeatureA", false); got != false {
		t.Errorf("expected failsafe false when server unreachable, got %v", got)
	}
}
