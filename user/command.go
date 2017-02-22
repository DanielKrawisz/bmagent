// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package user

import (
	"fmt"
	"strings"

	"github.com/DanielKrawisz/bmagent/keymgr/keys"
	"github.com/DanielKrawisz/bmagent/user/command"
	"github.com/DanielKrawisz/bmagent/user/email"
	"github.com/DanielKrawisz/bmutil/format"
	"github.com/DanielKrawisz/bmutil/identity"
	"github.com/jordwest/imap-server/types"
)

const (
	// MaxGenerateKeys is the maximum number of keys to be generated in one
	// command at a time.
	MaxGenerateKeys = 4000
)

var commands = make(map[string]func(*User, []string) (command.Response, error))

func init() {
	commands["generatekey"] = generateKeyCommand
}

// executeCommand executes an external command puts the result in
// our inbox as an email.
func (u *User) executeCommand(from, name, params string) error {
	// Get the inbox.
	commandFolder := u.boxes[CommandsFolderName]

	var r command.Response
	var err error

	// Check if such a command exists.
	cmd, ok := commands[name]
	if !ok {
		r = command.ErrorResponse(name, params, &command.ErrUnknown{name})
	} else { // Run the command.
		r, err = cmd(u, strings.Fields(params))
		if err != nil {
			r = command.ErrorResponse(name, params, err)
		}
	}

	// Email ourselves the response.
	err = commandFolder.AddNew(
		&email.Bmail{
			From:    "command@bm.agent",
			To:      from,
			Content: r.Email(),
		}, types.FlagRecent)
	if err != nil {
		return err
	}

	return nil
}

// GenerateKey creates n new keys for the user.
func (u *User) GenerateKey(n uint16, sendAck bool) []*keys.PrivateID {
	var behavior uint32

	if sendAck {
		behavior = identity.BehaviorAck
	}

	// first generate the new keys.
	var i uint16
	keyList := make([]*keys.PrivateID, 0, n)
	for i = 0; i < n; i++ {
		keyList = append(keyList, u.keys.NewUnnamed(DefaultStream, behavior))
	}

	return keyList
}

// generateKeyResponse represents a response to a GenerateKey command.
type generateKeyResponse struct {
	addrs []string
}

// generateKeyCommand creates n new keys for the user and sends him a message
// about them.
func generateKeyCommand(u *User, params []string) (command.Response, error) {
	var n uint64
	var err error
	var sendAck bool

	if len(params) > 2 {
		return nil, &command.ErrTooManyParameters{2}
	}

	// Use default value for sendAck.
	if len(params) < 2 {
		sendAck = true
	} else {
		err = command.ReadPattern(params[1], &sendAck)
		if err != nil {
			return nil, err
		}
	}

	// Use default value if no parameter given.
	if len(params) < 1 {
		n = 1
	} else {
		err = command.ReadPattern(params[0], &n)
		if err != nil {
			return nil, err
		}
	}

	if n > MaxGenerateKeys {
		return nil, &command.ErrValueTooBig{1, MaxGenerateKeys, n}
	}

	// generate the text containing the list of keys.
	addrs := make([]string, 0, n)
	for _, addr := range u.GenerateKey(uint16(n), sendAck) {
		addrs = append(addrs, fmt.Sprintf("\t%s@bm.addr\n", addr))
	}

	return &generateKeyResponse{
		addrs: addrs,
	}, nil
}

// Email converts the response to an email.
func (r *generateKeyResponse) Email() *format.Encoding2 {
	// generate the text containing the list of keys.
	keyList := ""
	for _, addr := range r.addrs {
		keyList = fmt.Sprint(keyList, fmt.Sprintf("\t%s@bm.addr\n", addr))
	}

	// Create the message.
	return &format.Encoding2{
		Subject: "New addresses generated.",
		Body:    fmt.Sprintf(newAddressesMsg, keyList),
	}
}

// RPC is the response formated as an RPC response.
func (r *generateKeyResponse) RPC() interface{} {
	return nil // Not yet implemented.
}
