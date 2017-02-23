// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package user

import (
	"strings"

	"github.com/DanielKrawisz/bmagent/cmd"
	"github.com/DanielKrawisz/bmutil"
	"github.com/DanielKrawisz/bmutil/identity"
	"github.com/jordwest/imap-server/types"
)

// executeCommand executes an external command puts the result in
// our inbox as an email.
func (u *User) executeCommand(from, name, params string) error {
	// Get the inbox.
	commandFolder := u.boxes[CommandsFolderName]

	bm, err := cmd.EmailCommand(u, from, "command@bm.agent", name, strings.Fields(params))

	if err != nil {
		return err
	}

	// Email ourselves the response.
	err = commandFolder.AddNew(bm, types.FlagRecent)
	if err != nil {
		return err
	}

	return nil
}

// NewAddress creates n new keys for the user.
func (u *User) NewAddress(tag string, sendAck bool) bmutil.Address {
	var behavior uint32

	if sendAck {
		behavior = identity.BehaviorAck
	}

	// first generate the new keys.
	return u.keys.NewUnnamed(DefaultStream, behavior).ToPublic().Address
}
