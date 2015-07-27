package main

import (
	//"time"

	"github.com/monetas/bmclient/keymgr"
	//"github.com/monetas/bmclient/rpc"
	"github.com/monetas/bmclient/store"
	//"github.com/monetas/bmutil"
	"github.com/monetas/bmutil/identity"
	//"github.com/monetas/bmutil/pow"
	//"github.com/monetas/bmutil/wire"
)

// addressBook implements the keymgr.AddressBook interface and
// manages the retrieval of identities from addresses.
type addressBook struct {
	managers []*keymgr.Manager
	addrs    map[string]*identity.Public
	//bmd      *rpc.Client
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
	serverLog.Trace("LookupPublicIdentity called on ", str)
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

	// Next ask bmd if it knows the public identity.
	/*identity, err = book.bmd.GetIdentity(str)
	if err == nil {
		book.addrs[str] = identity
		return identity, err
	}
	if err != rpc.ErrIdentityNotFound {
		return nil, err
	}

	// If all else fails, send a pubkey request over the network.
	// First check whether the address has already been requested.
	timestamp, err := book.pk.GetTime(str)
	if err == nil { // public key request has already been sent.
		if time.Now().Add(-1 * pubkeyExpiry).After(timestamp) {
			return nil, nil
		}
	}
	if err != store.ErrNotFound {
		return nil, err
	}

	// Generate a new pubky request.
	address, err := bmutil.DecodeAddress(str)
	if err != nil {
		return nil, err
	}
	var tag wire.ShaHash
	t := address.Tag()
	copy(tag[:], t)
	request := &wire.MsgGetPubKey{
		ObjectType:   wire.ObjectTypeGetPubKey,
		ExpiresTime:  time.Now().Add(pubkeyExpiry),
		Version:      4,
		StreamNumber: address.Stream,
		Tag:          &tag,
	}

	// Send it to the pow queue and insert it in the database.
	encoded := wire.EncodeMessage(request)
	target := pow.CalculateTarget(uint64(len(encoded)),
		uint64(request.ExpiresTime.Sub(time.Now()).Seconds()),
		pow.DefaultNonceTrialsPerByte,
		pow.DefaultExtraBytes)
	_, err = book.powQueue.Enqueue(target, encoded[8:])
	if err != nil {
		return nil, err
	}
	err = book.pk.New(str)
	if err != nil {
		return nil, err
	}

	// Nil with no error means that a pubkey request was sent.
	return nil, nil*/
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
