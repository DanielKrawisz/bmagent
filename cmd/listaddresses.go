package cmd

import (
	"github.com/DanielKrawisz/bmagent/cmd/rpc"
	//"github.com/DanielKrawisz/bmutil/identity"
)

type listAddressesResponse struct {
	addresses []PublicID
}

type listAddressesCommand struct{}

func (r *listAddressesCommand) Execute(u User) (Response, error) {
	return &listAddressesResponse{
		addresses: u.ListAddresses(),
	}, nil
}

func (r *listAddressesCommand) RPC() (*rpc.BMRPCRequest, error) {
	version := uint32(1)
	return &rpc.BMRPCRequest{
		Version: &version,
		Request: &rpc.BMRPCRequest_Listaddresses{
			Listaddresses: &rpc.ListAddressesRequest{
				Version: &version,
			},
		},
	}, nil
}

func readListAddressesCommand(param []string) (Command, error) {
	if len(param) != 0 {
		return nil, &ErrInvalidNumberOfParameters{
			params:   param,
			expected: 0,
		}
	}

	return &listAddressesCommand{}, nil
}

func buildListAddressesCommand(r *rpc.ListAddressesRequest) (Command, error) {
	return &listAddressesCommand{}, nil
}

var listAddresses = command{
	help: "list all available addresses",
	patterns: []Pattern{
		Pattern{
			key:  []Key{},
			help: "list all available addresses",
			read: readListAddressesCommand,
		},
	},
}

// String writes the help response as a string.
func (r *listAddressesResponse) String() string {
	return rpc.Message(r.RPC())
}

// RPC converts the response into an RPC protobuf message.
func (r *listAddressesResponse) RPC() *rpc.BMRPCReply {
	version := uint32(1)
	ids := make([]*rpc.BitmessageIdentity, len(r.addresses))
	for i, addr := range r.addresses {
		ids[i] = PublicToRPC(addr)
	}
	return &rpc.BMRPCReply{
		Version: &version,
		Reply: &rpc.BMRPCReply_Listaddresses{
			Listaddresses: &rpc.ListAddressesReply{
				Version:   &version,
				Addresses: ids,
			},
		},
	}
}
