package experiments

import (
	"context"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/local"
	"k8s.io/ingress-gce/pkg/experiments/grpc/pb"
	"k8s.io/klog/v2"
)

type GRPCClient struct {
	socketPath string
	logger     klog.Logger
	timeout    time.Duration
	client     pb.ExperimentsClient
	conn       *grpc.ClientConn
}

func NewGRPCClient(socketPath string, logger klog.Logger) (*GRPCClient, error) {
	return NewGRPCClientWithTimeout(socketPath, logger, 5*time.Second)
}

func NewGRPCClientWithTimeout(socketPath string, logger klog.Logger, timeout time.Duration) (*GRPCClient, error) {
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", socketPath)
	}

	conn, err := grpc.NewClient(
		"passthrough:///unix",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(local.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC client for unix socket %s: %w", socketPath, err)
	}

	pbClient := pb.NewExperimentsClient(conn)
	return &GRPCClient{
		socketPath: socketPath,
		logger:     logger,
		timeout:    timeout,
		client:     pbClient,
		conn:       conn,
	}, nil
}

func (g *GRPCClient) Close() error {
	if g.conn != nil {
		return g.conn.Close()
	}
	return nil
}

func (g *GRPCClient) BoolFlagValueOrFailsafe(ctx context.Context, flagName string, failSafeValue bool) bool {
	ctxWithTimeout, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	resp, err := g.client.BoolFlagValueOrFailsafe(ctxWithTimeout, &pb.BoolFlagRequest{
		FlagName:      flagName,
		FailsafeValue: failSafeValue,
	})

	if err != nil {
		g.logger.V(4).Info("gRPC call failed, falling back to failsafe", "flagName", flagName, "error", err, "failsafe", failSafeValue)
		return failSafeValue
	}

	return resp.GetValue()
}
