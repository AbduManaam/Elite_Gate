package main

import (
	"context"
	"fmt"
	"net"
	"os"

	pb "elitegate/api/proto/helloworld/v1"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

type greeterServer struct {
	pb.UnimplementedGreeterServer
}

func (s *greeterServer) SayHello(ctx context.Context, req *pb.HelloRequest) (*pb.HelloReply, error) {
	name := req.GetName()
	if name == "" {
		name = "world"
	}
	return &pb.HelloReply{Message: fmt.Sprintf("Hello %s from grpc-hello-service", name)}, nil
}

func main() {
	logger := zerolog.New(os.Stdout).With().Timestamp().Str("service", "grpc-hello").Logger()
	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		logger.Fatal().Err(err).Msg("listen failed")
	}

	s := grpc.NewServer()
	pb.RegisterGreeterServer(s, &greeterServer{})

	logger.Info().Str("addr", ":50052").Msg("grpc-hello-service listening")
	if err := s.Serve(lis); err != nil {
		logger.Fatal().Err(err).Msg("serve failed")
	}
}
