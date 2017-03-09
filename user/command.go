// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package user

import (
	"strings"

	"github.com/DanielKrawisz/bmagent/cmd"
	"github.com/DanielKrawisz/bmagent/idmgr/keys"
	"github.com/DanielKrawisz/bmutil"
	"github.com/DanielKrawisz/bmutil/identity"
	"github.com/jordwest/imap-server/types"
)

// PrivateIDToPublicID converts a PrivateID as returned from the key magager
// to a PublicID as expected by the command system.
func PrivateIDToPublicID(pi *keys.PrivateID) cmd.PublicID {
	return cmd.PublicID{
		ID:    pi.Private.Public(),
		Label: pi.Name,
	}
}

// executeCommand executes an external command puts the result in
// our inbox as an email.
func (u *User) executeCommand(from, name, params string) error {
	// Get the inbox.
	commandFolder := u.boxes[CommandsFolderName]

	bm := cmd.EmailCommand(u, from, "command@bm.agent", name, strings.Fields(params))

	// Email ourselves the response.
	err := commandFolder.AddNew(bm, types.FlagRecent)
	if err != nil {
		return err
	}

	return nil
}

// NewAddress creates n new keys for the user.
func (u *User) NewAddress(tag string, sendAck bool) cmd.PublicID {
	var behavior uint32

	if sendAck {
		behavior = identity.BehaviorAck
	}

	// first generate the new keys.
	return PrivateIDToPublicID(u.keys.NewUnnamed(bmutil.DefaultStream, behavior))
}

// ListAddresses lists all available addresses.
func (u *User) ListAddresses() []cmd.PublicID {
	names := u.keys.Names()
	pi := make([]cmd.PublicID, 0, len(names))
	for addr := range u.keys.Names() {
		pi = append(pi, PrivateIDToPublicID(u.keys.Get(addr)))
	}

	return pi
}
