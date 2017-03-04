package cmd

import (
	"github.com/DanielKrawisz/bmagent/cmd/rpc"
	"github.com/DanielKrawisz/bmutil"
)

// DefaultSendAck is set to true, which means that new addresses by default
// return ack messages.
const DefaultSendAck = true

type newAddressResponse struct {
	address bmutil.Address
}

type newAddressCommand struct {
	ack bool
	tag string // Currently unused, but should be used in the future.
}

func (r *newAddressCommand) Execute(u User) (Response, error) {
	return &newAddressResponse{
		address: u.NewAddress("", r.ack),
	}, nil
}

func (r *newAddressCommand) RPC() (*rpc.BMRPCRequest, error) {
	return nil, nil // TODO
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
	// TODO be sure to check that the version is always 4.
	return nil, &ErrUnimplemented{"newaddress"}
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
	return r.address.String()
}

func (r *newAddressResponse) RPC() *rpc.BMRPCReply {
	return nil // TODO
}
