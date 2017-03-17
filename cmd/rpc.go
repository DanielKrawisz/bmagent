package cmd

import (
	"errors"
	"fmt"
	"sync"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/DanielKrawisz/bmagent/bmrpc"
	pb "github.com/DanielKrawisz/bmagent/cmd/rpc"
	rpc "github.com/DanielKrawisz/bmd/rpc"
)

// ErrServerStopped is returned when an rpc call is made and the server
// isn't running.
var ErrServerStopped = errors.New("Server is not running.")

// BuildCommand translates an rpc request into a Command type.
func BuildCommand(r *pb.BMRPCRequest) (Command, error) {
	if r == nil || r.Request == nil || r.Version == nil || *r.Version != 1 {
		return nil, ErrInvalidRPCRequest
	}

	switch r := r.Request.(type) {
	default:
		return nil, ErrInvalidRPCRequest
	case *pb.BMRPCRequest_Help:
		return buildHelpCommand(r.Help)
	case *pb.BMRPCRequest_Newaddress:
		return buildNewAddressCommand(r.Newaddress)
	case *pb.BMRPCRequest_Listaddresses:
		return buildListAddressesCommand(r.Listaddresses)
	}
}

// RPCCommand manages a request sent via an rpc interface.
func RPCCommand(u User, request *pb.BMRPCRequest) (*pb.BMRPCReply, error) {
	rpcLog.Info("Received rpc command ", request.String())

	// Attempt to build a command from the request.
	command, err := BuildCommand(request)
	if err != nil {
		rpcLog.Info("Could not build command: ", err.Error())
		return nil, err
	}

	// Attempt to execute the command.
	response, err := command.Execute(u)
	if err != nil {
		rpcLog.Info("Could not build command: ", err.Error())
		return nil, err
	}

	return response.RPC(), nil
}

// RPCServer is a server that can listen and execute rpc commands.
type RPCServer struct {
	rpc.Server
	u     User
	mutex sync.Mutex
}

// BMAgentRequest returns the feature at the given point.
func (s *RPCServer) BMAgentRequest(ctx context.Context, req *pb.BMRPCRequest) (*pb.BMRPCReply, error) {
	s.Server.Lock()
	defer s.Server.Unlock()

	if !s.Server.Running() {
		return nil, ErrServerStopped
	}

	// TODO check signaturee.

	var reply *pb.BMRPCReply
	reply, err := RPCCommand(s.u, req)
	if err != nil {
		v := uint32(1)
		er := err.Error()
		return &pb.BMRPCReply{
			Version: &v,
			Reply: &pb.BMRPCReply_ErrorReply{
				ErrorReply: &pb.ErrorReply{
					Version: &v,
					Error:   &er,
				},
			},
		}, nil
	}

	// TODO sign the reply.

	return reply, nil
}

func newServer(u User, server *rpc.Server) *RPCServer {
	return &RPCServer{
		Server: *server,
		u:      u,
	}
}

// GRPCServer creates a grpc GRPC server.
func GRPCServer(u User, cfg *rpc.Config) (*RPCServer, error) {
	rpcLog.Info("Creating rpc server. cfg = ", cfg.String())

	rpcServer, err := rpc.NewRPCServer(cfg)
	if err != nil {
		return nil, err
	}

	bmaServer := newServer(u, rpcServer)

	pb.RegisterBMAgentRPCServer(rpcServer.GRPC(), bmaServer)

	return bmaServer, nil
}

type RPCClient struct {
	client pb.BMAgentRPCClient
}

func (r *RPCClient) Request(in *pb.BMRPCRequest) (*pb.BMRPCReply, error) {
	return r.client.BMAgentRequest(context.Background(), in)
}

// GRPCClient creates the GRPC client.
func GRPCClient(cfg *bmrpc.ClientConfig) (*RPCClient, error) {
	opts := []grpc.DialOption{grpc.WithTimeout(cfg.Timeout)}

	if cfg.DisableTLS {
		opts = append(opts, grpc.WithInsecure())
	} else {
		creds, err := credentials.NewClientTLSFromFile(cfg.CAFile, "")
		if err != nil {
			return nil, fmt.Errorf("Failed to create TLS credentials %v", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
	}

	conn, err := grpc.Dial(cfg.ConnectTo, opts...)

	if err != nil {
		return nil, fmt.Errorf("Failed to dial: %v", err)
	}
	bmd := pb.NewBMAgentRPCClient(conn)

	return &RPCClient{client: bmd}, err
}

// ØMQServer sets up the ØMQ Server
func ØMQServer() {
	// TODO
}
