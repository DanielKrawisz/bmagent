package cmd

import (
	"fmt"
	"strings"

	"github.com/DanielKrawisz/bmagent/user/email"
	"github.com/DanielKrawisz/bmutil/format"
)

// EmailCommand manages a request sent via the email interface.
func EmailCommand(u User, from, agent, name string, params []string) (*email.Bmail, error) {
	var r Response
	var err error

	// Check if such a command exists.
	command, ok := commands[name]
	if !ok {
		r = ErrorResponse(&ErrUnknownCommand{name})
	} else { // Run the command.
		r, err = executeEmail(u, command.patterns, params, name)
		if err != nil {
			r = ErrorResponse(err)
		}
	}

	return &email.Bmail{
		From: agent,
		To:   from,
		Content: &format.Encoding2{
			Subject: SubjectRe(name, params),
			Body:    r.String(),
		},
	}, nil
}

// SubjectRe creates the subject line of an email reply.
func SubjectRe(cmd string, p []string) string {
	return fmt.Sprintf("Re: %s %s", cmd, strings.Join(p, " "))
}

// execute runs one command sent via email.
func executeEmail(u User, patterns []Pattern, param []string, cmdName string) (Response, error) {

	// Go down the list of patterns and execute the first that matches.
	for _, p := range patterns {
		req, _ := p.read(param)
		if req == nil {
			continue
		}
		return req.Execute(u)
	}

	return nil, &ErrUnrecognizedPattern{
		command:            cmdName,
		recognizedPatterns: recognizedPatterns(patterns),
	}
}
