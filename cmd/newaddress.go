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

type newAddressRequest struct {
	ack bool
	tag string // Currently unused, but should be used in the future.
}

func (r *newAddressRequest) Execute(u User) (Response, error) {
	return &newAddressResponse{
		address: u.NewAddress("", r.ack),
	}, nil
}

func (r *newAddressRequest) RPC() (*rpc.BMRPCRequest, error) {
	return nil, nil // TODO
}

func readNewAddressRequest(param []string) (Request, error) {
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

	return &newAddressRequest{
		ack: sendAck,
	}, nil
}

func buildNewAddressRequest(r *rpc.BMRPCRequest) (Request, error) {
	// TODO be sure to check that the version is always 4.
	return nil, nil // TODO
}

var newAddressCommand = command{
	help: "creates a new address.",
	patterns: []Pattern{
		Pattern{
			key:   []Key{},
			help:  "Create a new unnamed address.",
			read:  readNewAddressRequest,
			proto: buildNewAddressRequest,
		},
		Pattern{
			key:   []Key{KeyBoolean},
			help:  "Create a new unnamed address.",
			read:  readNewAddressRequest,
			proto: buildNewAddressRequest,
		},
	},
}

// String writes the help response as a string.
func (r *newAddressResponse) String() string {
	addr, _ := r.address.Encode()
	return addr
}

func (r *newAddressResponse) RPC() *rpc.BMRPCReply {
	return nil // TODO
}
