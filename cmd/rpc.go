package cmd

import "github.com/DanielKrawisz/bmagent/cmd/rpc"

// RPCCommand manages a request sent via an rpc interface.
func RPCCommand(u User, request *rpc.BMRPCRequest) (*rpc.BMRPCReply, error) {
	return nil, nil // TODO
}

// executeProto
func executeProto(u User, patterns []Pattern, param *rpc.BMRPCRequest, cmdName string) (Response, error) {
	for _, p := range patterns {
		req, err := p.proto(param)
		if err != nil {
			return nil, err
		}
		return req.Execute(u)
	}

	return nil, &ErrUnrecognizedPattern{
		command:            cmdName,
		recognizedPatterns: recognizedPatterns(patterns),
	}
}
