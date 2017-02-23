package cmd

import (
	"github.com/DanielKrawisz/bmagent/cmd/rpc"
	"sort"
)

type command struct {
	help     string
	patterns []Pattern
}

// Request is the type created from the input from the user interface,
// either via email or via rpc.
type Request interface {
	Execute(User) (Response, error)
	RPC() (*rpc.BMRPCRequest, error)
}

// Response is the type returned by a command. It can be delivered to
// the user as a json object or as an email.
type Response interface {
	String() string
	RPC() *rpc.BMRPCReply
}

var unimplemented = command{
	help: "has not yet been implemented.",
}

// CommandList contains all command names in alphabetical order.
var CommandList []string

var commands = make(map[string]command)

func init() {
	commands["help"] = helpCommand
	commands["newaddress"] = newAddressCommand
	commands["getmessages"] = unimplemented
	commands["deletemessages"] = unimplemented
	commands["send"] = unimplemented
	commands["subscribe"] = unimplemented

	CommandList = make([]string, 0, len(commands))
	for name := range commands {
		CommandList = append(CommandList, name)
	}
	sort.Strings(CommandList)
}
