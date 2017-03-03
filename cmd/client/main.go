package main

import (
	"os"
	
	"github.com/DanielKrawisz/bmagent/cmd"
)

func run(cmdName string, args []string) error {
	// Check if the command exists. 
	command, err := cmd.ReadCommand(cmdName, args)
	if err != nil {
		return err
	}
	
	// Make an rpc request out of the command. 
	req, err := command.RPC()
	if err != nil {
		return err
	}
	
	// Translate the rpc request back into a command. (In the future, 
	// this step will actually query bmagent via rpc.)
	command, err = cmd.BuildCommand(req)
	if err != nil {
		return err
	}
	
	command.Execute(&mockUser{})
	
	return nil
}

func main() {
	if len(os.Args) < 2 {
		println("Put a help message here.")
	}
	
	name := os.Args[1]
	args := os.Args[2:]
	
	err := run(name, args)
	if err != nil {
		println(err.Error())
	}
}