package main

import (
	"os"

	"github.com/DanielKrawisz/bmagent/cmd"
	"github.com/DanielKrawisz/bmagent/cmd/rpc"
)

func run(cmdName string, args []string) (*rpc.BMRPCReply, error) {
	// Check if the command exists.
	command, err := cmd.ReadCommand(cmdName, args)
	if err != nil {
		return nil, err
	}

	// Make an rpc request out of the command.
	req, err := command.RPC()
	if err != nil {
		return nil, err
	}

	// Translate the rpc request back into a command. (In the future,
	// this step will actually query bmagent via rpc.)
	command, err = cmd.BuildCommand(req)
	if err != nil {
		return nil, err
	}

	response, err := command.Execute(&mockUser{})
	if err != nil {
		return nil, err
	}

	return response.RPC(), nil
}

func main() {
	if len(os.Args) < 2 {
		println("Put a help message here.")
		return
	}

	name := os.Args[1]
	args := os.Args[2:]

	response, err := run(name, args)
	if err != nil {
		println(err.Error())
		return
	}

	println(rpc.Message(response))
}
