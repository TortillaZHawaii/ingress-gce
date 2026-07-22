package grpc

import (
	"context"
	"fmt"
	"net"
	"os"

	"google.golang.org/grpc"
	"k8s.io/ingress-gce/pkg/experiments"
	"k8s.io/ingress-gce/pkg/experiments/grpc/pb"
	"k8s.io/klog/v2"
)

type Server struct {
	pb.UnimplementedExperimentsServer
	client     experiments.Client
	logger     klog.Logger
	grpcServer *grpc.Server
}

func NewServer(client experiments.Client, logger klog.Logger) *Server {
	return &Server{
		client: client,
		logger: logger,
	}
}

func (s *Server) BoolFlagValueOrFailsafe(ctx context.Context, req *pb.BoolFlagRequest) (*pb.BoolFlagResponse, error) {
	val := s.client.BoolFlagValueOrFailsafe(ctx, req.GetFlagName(), req.GetFailsafeValue())
	return &pb.BoolFlagResponse{Value: val}, nil
}

func (s *Server) StartOnUnixSocket(socketPath string) (net.Listener, error) {
	_ = os.Remove(socketPath)

	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on unix socket %s: %w", socketPath, err)
	}

	s.grpcServer = grpc.NewServer()
	pb.RegisterExperimentsServer(s.grpcServer, s)

	go func() {
		if err := s.grpcServer.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			s.logger.Error(err, "gRPC server error")
		}
	}()

	return lis, nil
}

func (s *Server) Stop() {
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
}
