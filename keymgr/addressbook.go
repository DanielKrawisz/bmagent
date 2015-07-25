// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package keymgr

import (
	"github.com/monetas/bmutil/identity"
)

type AddressBook interface {
	// LookupPublicIdentity attempts to retreive a public identity
	// given an address.
	// If the function returns nil with no error, that means
	// that a pubkey request was sent.
	LookupPublicIdentity(string) (*identity.Public, error)
	// AddPublicIdentity adds a public identity to the cache.
	AddPublicIdentity(string, *identity.Public)
	// LookupPrivateIdentity checks the key manager for the right private key.
	LookupPrivateIdentity(string) (*PrivateID, error)
}
