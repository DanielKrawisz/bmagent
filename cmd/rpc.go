package cmd

import (
	"fmt"
	"net"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/DanielKrawisz/bmagent/cmd/rpc"
)

// BuildCommand translates an rpc request into a Command type.
func BuildCommand(r *rpc.BMRPCRequest) (Command, error) {
	if r == nil || r.Request == nil || r.Version == nil || *r.Version != 1 {
		return nil, ErrInvalidRPCRequest
	}

	switch r := r.Request.(type) {
	default:
		return nil, ErrInvalidRPCRequest
	case *rpc.BMRPCRequest_Help:
		return buildHelpCommand(r.Help)
	case *rpc.BMRPCRequest_Newaddress:
		return buildNewAddressCommand(r.Newaddress)
	case *rpc.BMRPCRequest_Listaddresses:
		return buildListAddressesCommand(r.Listaddresses)
	}
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

// RPCServer is a server that can listen and execute rpc commands.
type RPCServer struct {
	u User
}

// BMAgentRequest returns the feature at the given point.
func (s *RPCServer) BMAgentRequest(ctx context.Context, req *rpc.BMRPCRequest) (*rpc.BMRPCReply, error) {
	// TODO check signaturee.

	var reply *rpc.BMRPCReply
	reply, err := RPCCommand(s.u, req)
	if err != nil {
		v := uint32(1)
		er := err.Error()
		return &rpc.BMRPCReply{
			Version: &v,
			Reply: &rpc.BMRPCReply_ErrorReply{
				ErrorReply: &rpc.ErrorReply{
					Version: &v,
					Error:   &er,
				},
			},
		}, nil
	}

	// TODO sign the reply.

	return reply, nil
}

func newServer(u User) *RPCServer {
	return &RPCServer{
		u: u,
	}
}

// GRPCServer creates and starts a- GRPC server.
func GRPCServer(u User, port uint32) (*RPCServer, error) {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %v", err)
	}

	rpcServer := newServer(u)
	grpcServer := grpc.NewServer(nil)
	rpc.RegisterBMAgentRPCServer(grpcServer, rpcServer)
	grpcServer.Serve(lis)

	return rpcServer, nil
}

// ØMQServe sets up the ØMQ Server
func ØMQServe() {
	// TODO
}
