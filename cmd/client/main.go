package main

import (
	"net"
	"os"
	"time"

	"github.com/DanielKrawisz/bmagent/bmrpc"
	"github.com/DanielKrawisz/bmagent/cmd"
	"github.com/DanielKrawisz/bmagent/cmd/rpc"
)

func request(req *rpc.BMRPCRequest) (*rpc.BMRPCReply, error) {
	// Create an rpc connection to bmagent.
	client, err := cmd.GRPCClient(&bmrpc.ClientConfig{
		DisableTLS: true,
		ConnectTo:  net.JoinHostPort("localhost", "8446"),
		Timeout:    time.Second,
	})
	if err != nil {
		println("request error...")
		return nil, err
	}

	return client.Request(req)
}

func run(command cmd.Command, args []string) (*rpc.BMRPCReply, error) {
	// Make an rpc request out of the command.
	req, err := command.RPC()
	if err != nil {
		return nil, err
	}

	// Send it to the server.
	return request(req)
}

func try(cmdName string, args []string) string {
	// Check if the command exists.
	command, err := cmd.ReadCommand(cmdName, args)
	if err != nil {
		return err.Error()
	}

	// Try to send it to the server.
	response, err := run(command, args)
	if err != nil {
		println("response error")
		return err.Error()
	}

	return rpc.Message(response)
}

func main() {
	if len(os.Args) < 2 {
		println("Put a help message here.")
		return
	}

	name := os.Args[1]
	args := os.Args[2:]

	println(try(name, args))
}
