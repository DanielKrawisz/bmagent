package cmd

import (
	"fmt"
	"net"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/DanielKrawisz/bmagent/cmd/rpc"
)

// BuildCommand translates a rpc request into a Command type.
func BuildCommand(r *rpc.BMRPCRequest) (Command, error) {
	return nil, ErrInvalidRPCRequest // TODO
}

// RPCCommand manages a request sent via an rpc interface.
func RPCCommand(u User, request *rpc.BMRPCRequest) (*rpc.BMRPCReply, error) {
	// Attempt to build a command from the request.
	command, err := BuildCommand(request)
	if err != nil {
		return nil, err
	}

	// Attempt to execute the command.
	response, err := command.Execute(u)
	if err != nil {
		return nil, err
	}

	return response.RPC(), nil
}

type grpcServer struct {
	u User
}

// GetFeature returns the feature at the given point.
func (s *grpcServer) BMAgentRequest(ctx context.Context, req *rpc.BMRPCRequest) (*rpc.BMRPCReply, error) {
	// TODO check signaturee.

	var reply *rpc.BMRPCReply
	reply, err := RPCCommand(s.u, req)
	if err != nil {
		// TODO make error reply for this.
	}

	// TODO sign the reply.

	return reply, nil
}

func newServer(u User) *grpcServer {
	s := grpcServer{
		u: u,
	}
	return &s
}

// GRPCServe sets up the GRPC server.
func GRPCServe(u User, port uint32) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer(nil)
	rpc.RegisterBMAgentRPCServer(grpcServer, newServer(u))
	grpcServer.Serve(lis)

	return nil
}

// ØMQServe sets up the ØMQ Server
func ØMQServe() {
	// TODO
}