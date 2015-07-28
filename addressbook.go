package main

import (
	"github.com/monetas/bmclient/keymgr"
	"github.com/monetas/bmclient/store"
	"github.com/monetas/bmutil/identity"
)

// addressBook implements the keymgr.AddressBook interface and
// manages the retrieval of identities from addresses.
type addressBook struct {
	managers []*keymgr.Manager
	addrs    map[string]*identity.Public
	pk       *store.PKRequests
	powQueue *store.PowQueue
	server   *server
}

func (book *addressBook) AddPublicIdentity(str string, identity *identity.Public) {
	book.addrs[str] = identity
}

// LookupPublicIdentity looks for the public identity of an address. If it can't
// find one, it sends out a get pubkey request. If both return values are nil,
// this means that a get pubkey request was sent.
func (book *addressBook) LookupPublicIdentity(str string) (*identity.Public, error) {
	// Check the map of cached identities.
	identity, ok := book.addrs[str]
	if ok {
		return identity, nil
	}

	// Check the private identities, just in case.
	private, err := book.LookupPrivateIdentity(str)
	if err == nil {
		return private.ToPublic(), nil
	}
	if err != keymgr.ErrNonexistentIdentity {
		return nil, err
	}

	return book.server.getOrRequestPublicIdentity(str)
}

// LookupPrivateIdentity searches for the private identity of an address.
func (book *addressBook) LookupPrivateIdentity(str string) (*keymgr.PrivateID, error) {
	for _, manager := range book.managers {
		identity, _ := manager.LookupByAddress(str)
		if identity != nil {
			return identity, nil
		}
	}

	return nil, keymgr.ErrNonexistentIdentity
}
