// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"time"

	"github.com/DanielKrawisz/bmagent/keymgr"
	"github.com/DanielKrawisz/bmagent/store"
	"github.com/DanielKrawisz/bmutil/identity"
	"github.com/DanielKrawisz/bmutil/pow"
	"github.com/DanielKrawisz/bmutil/wire"
)

// serverOps implements the email.ServerOps interface.
type serverOps struct {
	pubIDs map[string]*identity.Public // a cache
	id     uint32
	user   *User
	data   *store.UserData
	server *server
}

// GetOrRequestPublic attempts to retreive a public identity for the given
// address. If the function returns nil with no error, that means that a pubkey
// request was successfully queued for proof-of-work.
func (s *serverOps) GetOrRequestPublicID(addr string) (*identity.Public, error) {
	serverLog.Debug("GetOrRequestPublicID for ", addr)

	// Check the map of cached identities.
	identity, ok := s.pubIDs[addr]
	if ok {
		serverLog.Debug("GetOrRequestPublicID: identity found ", addr)
		return identity, nil
	}

	// Check the private identities, just in case.
	private := s.GetPrivateID(addr)
	if private != nil {
		return private.ToPublic(), nil
	}

	pubID, err := s.server.getOrRequestPublicIdentity(s.id, addr)
	if err != nil { // Some error occured.
		return nil, err
	}
	if pubID == nil && err == nil { // Getpubkey request sent.
		return nil, nil
	}

	s.pubIDs[addr] = pubID
	return pubID, nil
}

// GetPrivateID queries the key manager for the right private key for the given
// address.
func (s *serverOps) GetPrivateID(addr string) *keymgr.PrivateID {
	return s.user.Keys.LookupByAddress(addr)
}

// GetObjectExpiry returns the time duration after which an object of the
// given type will expire on the network. It's used for POW calculations.
func (s *serverOps) GetObjectExpiry(objType wire.ObjectType) time.Duration {
	switch objType {
	case wire.ObjectTypeMsg:
		return cfg.MsgExpiry
	case wire.ObjectTypeBroadcast:
		return cfg.BroadcastExpiry
	case wire.ObjectTypeGetPubKey:
		return defaultGetpubkeyExpiry
	case wire.ObjectTypePubKey:
		return defaultPubkeyExpiry
	default:
		return defaultUnknownObjExpiry
	}
}

// PowQueue returns the store.PowQueue associated with the server.
func (s *serverOps) RunPow(target uint64, obj []byte, done func(n pow.Nonce)) {
	s.server.pow.Run(target, obj, done)
}

func (s *serverOps) Send(obj []byte) { // Send the object out on the network.
	s.server.Send(obj)
}

// Folders returns the set of folders for a given user.
func (s *serverOps) Folders() []store.Folder {
	return s.data.Folders()
}
