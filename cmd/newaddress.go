package cmd

import (
	"errors"
	"github.com/DanielKrawisz/bmagent/cmd/rpc"
	"github.com/DanielKrawisz/bmutil/identity"
)

// DefaultSendAck is set to true, which means that new addresses by default
// return ack messages.
const DefaultSendAck = true

// PublicID is a public identity with a name.
type PublicID struct {
	ID    identity.Public
	Label string
}

type newAddressResponse struct {
	address PublicID
}

type newAddressCommand struct {
	ack   bool
	label string // Currently unused, but should be used in the future.
}

func (r *newAddressCommand) Execute(u User) (Response, error) {
	addr := u.NewAddress("", r.ack)
	return &newAddressResponse{
		address: addr,
	}, nil
}

func (r *newAddressCommand) RPC() (*rpc.BMRPCRequest, error) {
	version := uint32(1)
	return &rpc.BMRPCRequest{
		Version: &version,
		Request: &rpc.BMRPCRequest_Newaddress{
			Newaddress: &rpc.NewAddressRequest{
				Version: &version,
				Label:   &r.label,
			},
		},
	}, nil
}

func readNewAddressCommand(param []string) (Command, error) {
	var sendAck bool
	var err error
	switch len(param) {
	default:
		return nil, &ErrInvalidNumberOfParameters{
			params:   param,
			expected: 1,
		}
	case 0:
		sendAck = DefaultSendAck
	case 1:
		err = ReadPattern(param, &sendAck)
		if err != nil {
			return nil, err
		}
	}

	return &newAddressCommand{
		ack: sendAck,
	}, nil
}

func buildNewAddressCommand(r *rpc.NewAddressRequest) (Command, error) {
	if *r.Addressversion != uint32(4) {
		return nil, errors.New("Address version must be 4")
	}

	return &newAddressCommand{
		ack:   *r.Ack,
		label: *r.Label,
	}, nil
}

var newAddress = command{
	help: "creates a new address.",
	patterns: []Pattern{
		Pattern{
			key:  []Key{},
			help: "Create a new unnamed address.",
			read: readNewAddressCommand,
		},
		Pattern{
			key:  []Key{KeyBoolean},
			help: "Create a new unnamed address.",
			read: readNewAddressCommand,
		},
	},
}

// String writes the help response as a string.
func (r *newAddressResponse) String() string {
	return rpc.Message(r.RPC())
}

// RPC converts the response into an RPC protobuf message.
func (r *newAddressResponse) RPC() *rpc.BMRPCReply {
	version := uint32(1)
	return &rpc.BMRPCReply{
		Version: &version,
		Reply: &rpc.BMRPCReply_Newaddress{
			Newaddress: &rpc.NewAddressReply{
				Version: &version,
				Address: PublicToRPC(r.address),
			},
		},
	}
}

// PublicToRPC converts an identity.Public to a BitmessageIdentity
func PublicToRPC(ip PublicID) *rpc.BitmessageIdentity {
	version := uint32(1)
	str := ip.ID.Address().String()
	b := ip.ID.Behavior()
	pow := ip.ID.Pow()
	return &rpc.BitmessageIdentity{
		Version:            &version,
		Address:            &str,
		Behavior:           &b,
		Noncetrialsperbyte: &pow.NonceTrialsPerByte,
		Extrabytes:         &pow.ExtraBytes,
		Label:              &ip.Label,
	}
}
