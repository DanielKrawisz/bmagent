package cmd

import (
	"fmt"
	"strings"

	"github.com/DanielKrawisz/bmagent/user/email"
	"github.com/DanielKrawisz/bmutil/format"
)

// EmailCommand manages a request sent via the email interface.
func EmailCommand(u User, from, agent, name string, params []string) *email.Bmail {
	var r Response

	// Find a command corresponding to the name.
	if command, err := ReadCommand(name, params); err != nil {
		r = ErrorResponse(err)
		// Attempt to execute it.
	} else if r, err = command.Execute(u); err != nil {
		r = ErrorResponse(err)
	}

	return &email.Bmail{
		From: agent,
		To:   from,
		Content: &format.Encoding2{
			Subject: SubjectRe(name, params),
			Body:    r.String(),
		},
	}
}

// SubjectRe creates the subject line of an email reply.
func SubjectRe(cmd string, p []string) string {
	return fmt.Sprintf("Re: %s %s", cmd, strings.Join(p, " "))
}
