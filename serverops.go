// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"time"

	"github.com/DanielKrawisz/bmagent/keymgr/keys"
	"github.com/DanielKrawisz/bmagent/user"
	"github.com/DanielKrawisz/bmagent/user/email"
	"github.com/DanielKrawisz/bmutil/identity"
	"github.com/DanielKrawisz/bmutil/wire"
)

// serverOps implements the email.ServerOps interface.
type serverOps struct {
	pubIDs map[string]identity.Public // a cache
	id     uint32
	user   *User
	server *server
}

// GetOrRequestPublic attempts to retreive a public identity for the given
// address. If the function returns nil with no error, that means that a pubkey
// request was successfully queued for proof-of-work.
func (s *serverOps) GetOrRequestPublicID(emailAddress string) (identity.Public, error) {
	addr, err := email.ToBm(emailAddress)
	if err != nil {
		return nil, err
	}

	if addr == email.Broadcast {
		return user.Broadcast, nil
	}

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
		return private.Public(), nil
	}

	pubID, err := s.server.getOrRequestPublicIdentity(s.id, addr)
	if err != nil { // Some error occured.
		return nil, err
	}
	if pubID == nil && err == nil { // Getpubkey request sent.
		return nil, email.ErrGetPubKeySent
	}

	s.pubIDs[addr] = pubID
	return pubID, nil
}

// GetPrivateID queries the key manager for the right private key for the given
// address.
func (s *serverOps) GetPrivateID(addr string) *keys.PrivateID {
	return s.user.Keys.Get(addr)
}

// ObjectExpiration returns the time duration after which an object of the
// given type will expire on the network. It's used for POW calculations.
func ObjectExpiration(objType wire.ObjectType) time.Duration {
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

func (s *serverOps) Send(obj []byte) { // Send the object out on the network.
	s.server.Send(obj)
}
