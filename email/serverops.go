// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

import (
	"time"

	"github.com/monetas/bmclient/keymgr"
	"github.com/monetas/bmclient/store"
	"github.com/monetas/bmutil/identity"
	"github.com/monetas/bmutil/wire"
)

// ServerOps is used for doing operations best performed by the server and its
// components. This includes requesting public and private identities from the
// server and accessing some config options.
type ServerOps interface {
	// GetOrRequestPublicID attempts to retreive a public identity for the given
	// address. If the function returns nil with no error, that means that a
	// pubkey request was successfully queued for proof-of-work.
	GetOrRequestPublicID(string) (*identity.Public, error)

	// GetPrivateID queries the key manager for the right private key for the
	// given address.
	GetPrivateID(string) (*keymgr.PrivateID, error)

	// GetObjectExpiry returns the time duration after which an object of the
	// given type will expire on the network. It's used for POW calculations.
	GetObjectExpiry(wire.ObjectType) time.Duration

	// RunPow submits some data to have the pow calculated and submitted to the network.
	RunPow(uint64, []byte) (uint64, error)

	// Mailboxes returns the set of mailboxes in the store.
	Mailboxes() []*store.Mailbox
}
