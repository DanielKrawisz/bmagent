// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"

	"github.com/DanielKrawisz/bmagent/cmd/rpc"
)

// ***************** Available Commands *****************
//
// The following commands are fully implemented according to the steps listed
// in the next section.

// Commands is the list of commands.
var Commands = []string{
	"deletemessages",
	"getmessages",
	"help",
	"listaddresses",
	"newaddress",
	"sendmessage",
}

// The following commands are partially implemented.
//
//   (none)
//
// The following commands are planned to be implemented in the future.

// Unimplemented is the list of unimplemented commands.
var Unimplemented = []string{
	"deletemessages",
	"getmessages",
	"listaddresses",
	"newaddress",
	"sendmessage",
}

// ******************** INSTRUCTIONS ********************
//
// How to Create a Command
//
//   Definitions:
//     dosomething := the command you want to implement.
//
//   1. Define DoSomethingCommand and DoSomethingReply in cmd/rpc/rpc.proto.
//      All protobuf message definitions must include an optional uint32
//      version.
//   2. Make a file called dosomething.go in cmd.
//   3. Make a member function called Message in rpc/proto.go for the
//      protobuf object that I just created.
//   4. Make a struct called doSomethingResponse which implements Response.

// Response is the type returned by a command. It can be delivered to
// the user as a json object or as an email.
type Response interface {
	String() string
	RPC() *rpc.BMRPCReply
}

//   5. Make a struct called doSomethingCommand which implements Command.

// Command is the type created from the input through the user interface,
// either via email or via rpc.
type Command interface {
	Execute(User) (Response, error)
	RPC() (*rpc.BMRPCRequest, error)
}

//   6. Make a function called readDoSomethingCommand which translates a
//      parameter list provided by email into a doSomethingCommand. If there
//      are multiple patterns of parameter lists which make an acceptable
//      request, you may make multiple functions called doSomethingCommandX,
//      where X is the name of the pattern.
//   7. Make a function called buildDoSomethingCommand which takes an
//      protobuf Command and translates it into a doSomethingCommand.
//   8. Make an instance of command called doSomething which includes a help
//      message and a list of pattern objects. init() insures that any program
//      using this package will panic if not all help messages are provided.

type command struct {
	help     string
	patterns []Pattern
}

//   9. Add dosomething to Commands (see above).
//  10. In command.go, add a line to init() which adds a dosomething entry to
//      commands.

var commands = make(map[string]command)

var unimplemented = make(map[string]command)

// A stub which can be used for unimplemented commands which do not
// even have a proper help message yet.
var unimplementedStub = command{
	help: "has not yet been implemented.",
}

func init() {
	commands["help"] = help
	commands["newaddress"] = newAddress
	commands["getmessages"] = unimplementedStub
	commands["deletemessages"] = unimplementedStub
	commands["sendmessage"] = unimplementedStub
	commands["subscribe"] = unimplementedStub
	commands["listaddresses"] = unimplementedStub

	// Ensure that Commands is in alphabetical order and every element
	// in Commands is in commands.
	commandsSet := make(map[string]struct{})
	{
		previous := ""
		for _, command := range Commands {
			if previous > command {
				panic(fmt.Sprint("Commands is not in alapbetical order ", previous, " > ", command))
			}

			previous = command

			if _, ok := commands[command]; !ok {
				panic(fmt.Sprint("Command ", command, " is not in commands"))
			}

			commandsSet[command] = struct{}{}
		}
	}

	// Ensure that Unimplemented is in alphabetical order and
	// that the same command does not appear in both.
	{
		previous := ""
		for _, command := range Unimplemented {
			if previous > command {
				panic(fmt.Sprint("Unimplemented is not in alphabetical order ", previous, " > ", command))
			}

			previous = command

			if _, ok := commandsSet[command]; !ok {
				panic(fmt.Sprint("Unimplemented command ", command, " is not in Commands"))
			}
		}
	}

	// Ensure that every command and every pattern has a help message.
	for _, command := range commands {
		if command.help == "" {
			panic(fmt.Sprint("Command ", command, " has no help message"))
		}

		for i, pattern := range command.patterns {
			if pattern.help == "" {
				panic(fmt.Sprint("Command ", command, ", pattern ", i, " has no help message"))
			}
		}
	}
}

// ReadCommand attempts to take a command name and list of parameters and
// translates it into a Command type.
func ReadCommand(name string, param []string) (Command, error) {
	// Check if such a command exists.
	command, ok := commands[name]
	if !ok {
		return nil, &ErrUnknownCommand{name}
	}

	// Go down the list of patterns and return the first that matches.
	for _, p := range command.patterns {
		c, _ := p.read(param)
		if c == nil {
			continue
		}
		return c, nil
	}

	return nil, &ErrUnrecognizedPattern{
		command:            name,
		recognizedPatterns: recognizedPatterns(command.patterns),
	}
}
