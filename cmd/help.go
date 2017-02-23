package cmd

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/DanielKrawisz/bmagent/cmd/rpc"
)

type helpResponse struct {
	commands []string
	long     bool
}

type helpRequest struct {
	commands []string
}

// Execute executes the help request using the given user.
func (r *helpRequest) Execute(u User) (Response, error) {
	if len(r.commands) == 0 {
		return &helpResponse{
			commands: CommandList,
			long:     false,
		}, nil
	}

	return &helpResponse{
		commands: r.commands,
		long:     true,
	}, nil
}

// RPC transforms the Response into the protobuf reply.
func (r *helpRequest) RPC() (*rpc.BMRPCRequest, error) {
	return nil, nil // TODO
}

func readHelpRequest(param []string) (Request, error) {
	for _, p := range param {
		_, ok := commands[p]
		if !ok {
			return nil, &ErrUnknownCommand{p}
		}
	}

	sort.Strings(param)
	return &helpRequest{
		commands: param,
	}, nil
}

func buildHelpRequest(r *rpc.BMRPCRequest) (Request, error) {
	return nil, nil // TODO
}

var helpCommand = command{
	help: "provides instructions on commands.",
	patterns: []Pattern{
		Pattern{
			key:   []Key{},
			help:  "Print instructions for all commands.",
			read:  readHelpRequest,
			proto: buildHelpRequest,
		},
		Pattern{
			key:   []Key{KeySymbol, KeyRepeated},
			help:  "Print instructions for specified commands.",
			read:  readHelpRequest,
			proto: buildHelpRequest,
		},
	},
}

// String writes the response as a string.
func (r *helpResponse) String() string {
	var b bytes.Buffer
	var write func(b *bytes.Buffer, cmd string, command command)
	if r.long {
		write = helpMessageLong
	} else {
		write = helpMessageShort
	}

	for _, cmdName := range r.commands {

		command := commands[cmdName]

		write(&b, cmdName, command)
	}

	return b.String()
}

// RPC transforms the Response into the protobuf reply.
func (r *helpResponse) RPC() *rpc.BMRPCReply {
	return nil // TODO
}

// helpMessageShort writes a short help message for a command.
func helpMessageShort(b *bytes.Buffer, cmd string, command command) {
	b.Write([]byte(fmt.Sprintf("  %-14s : ", cmd)))
	formatHelp(b, command.help, 4, 19, 78)
}

// helpMessageLong writes the full help message for a command.
func helpMessageLong(b *bytes.Buffer, cmd string, command command) {
	helpMessageShort(b, cmd, command)
	for _, pat := range command.patterns {
		b.Write([]byte("\n"))
		helpMessagePattern(b, cmd, pat)
	}
}

// helpMessagePattern writes the help message for a pattern.
func helpMessagePattern(b *bytes.Buffer, cmd string, pattern Pattern) {
	b.Write([]byte(fmt.Sprintf("  %-14s %s\n", cmd, patternString(pattern.key))))
	formatHelp(b, pattern.help, 4, 4, 78)
}

// formatHelp writes a help message with formatting.
func formatHelp(b *bytes.Buffer, help string, indent, firstIndent, lineWidth uint32) {
	// TODO
}
