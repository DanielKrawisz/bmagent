package cmd

import (
	"errors"
	"github.com/DanielKrawisz/bmagent/cmd/rpc"
	"github.com/DanielKrawisz/bmutil/identity"
	"github.com/DanielKrawisz/bmutil/pow"
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
	ack     bool
	label   *string // Currently unused, but should be used in the future.
	pow     *pow.Data
	version uint64
	stream  uint64
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
				Label:   r.label,
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
		ack:     sendAck,
		version: 4,
		stream:  1,
	}, nil
}

func buildNewAddressCommand(r *rpc.NewAddressRequest) (Command, error) {
	if r.Addressversion != nil && *r.Addressversion != uint32(4) {
		return nil, errors.New("Address version must be 4")
	}
	if r.Stream != nil && *r.Stream != uint32(1) {
		return nil, errors.New("Stream must be 1")
	}

	var ack bool
	if r.Ack != nil {
		ack = *r.Ack
	} else {
		ack = true
	}

	var p pow.Data
	if r.Noncetrialsperbyte != nil {
		if r.Extrabytes != nil {
			return nil, errors.New("Must provide both nonce trials per byte and extra bytes.")
		}

		p = pow.Data{uint64(*r.Noncetrialsperbyte), uint64(*r.Extrabytes)}
	}

	return &newAddressCommand{
		ack:     ack,
		label:   r.Label,
		pow:     &p,
		version: 4,
		stream:  1,
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
			help: "Create a new named address.",
			read: readNewAddressCommand,
		},
		Pattern{
			key:  []Key{KeyBoolean, KeyNatural, KeyNatural},
			help: "Create a new named address.",
			read: readNewAddressCommand,
		},
		Pattern{
			key:  []Key{KeyString},
			help: "Create a new named address.",
			read: readNewAddressCommand,
		},
		Pattern{
			key:  []Key{KeyString, KeyBoolean},
			help: "Create a new named address.",
			read: readNewAddressCommand,
		},
		Pattern{
			key:  []Key{KeyString, KeyBoolean, KeyNatural, KeyNatural},
			help: "Create a new named address.",
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
