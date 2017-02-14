// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package user

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/rand"
	"time"

	"github.com/DanielKrawisz/bmagent/store"
	"github.com/DanielKrawisz/bmagent/user/email"
	"github.com/DanielKrawisz/bmutil/cipher"
	"github.com/DanielKrawisz/bmutil/format"
	"github.com/DanielKrawisz/bmutil/identity"
	"github.com/DanielKrawisz/bmutil/pow"
	"github.com/DanielKrawisz/bmutil/wire"
	"github.com/DanielKrawisz/bmutil/wire/obj"
)

// ServerOps is used for doing operations best performed by the server and its
// components. This includes requesting public and private identities from the
// server and accessing some config options.
type ServerOps interface {
	// GetOrRequestPublicID attempts to retreive a public identity for the given
	// address. If the function returns nil with no error, that means that a
	// pubkey request was successfully queued for proof-of-work.
	GetOrRequestPublicID(string) (*identity.Public, error)

	// RunPow submits some data to have the pow calculated and submitted to the network.
	RunPow(uint64, []byte, func(n pow.Nonce))

	// Mailboxes returns the set of mailboxes in the store.
	Folders() []store.Folder

	// Send sends a message out into the network.
	Send(obj []byte)
}

// generateBroadcast generates a wire.MsgBroadcast from a Bitmessage.
func generateBroadcast(content format.Encoding, from *identity.Private, expiry time.Duration) (obj.Object, *obj.PubKeyData, error) {
	pkd := from.ToPubKeyData()

	var tag wire.ShaHash
	copy(tag[:], from.Address.Tag())

	data := &cipher.Bitmessage{
		FromStreamNumber:   from.Address.Stream,
		FromAddressVersion: from.Address.Version,
		SigningKey:         pkd.VerificationKey,
		EncryptionKey:      pkd.EncryptionKey,
		Pow:                pkd.Pow,
		Content:            content,
	}

	var broadcast *cipher.Broadcast
	var err error
	switch from.Address.Version {
	case 2:
		fallthrough
	case 3:
		broadcast, err = cipher.CreateTaglessBroadcast(time.Now().Add(expiry), data, from)
	case 4:
		broadcast, err = cipher.CreateTaggedBroadcast(time.Now().Add(expiry), data, &tag, from)
	default:
		return nil, nil, errors.New("Unknown from address version")
	}

	if err != nil {
		return nil, nil, err
	}

	return broadcast.Object(), pkd, nil
}

// generateMsg generates a cipher.Message from a Bitmessage.
func generateMessage(content format.Encoding, ack []byte, from *identity.Private, to *identity.Public, expiry time.Duration) (obj.Object, *obj.PubKeyData, error) {
	pkd := from.ToPubKeyData()

	var destination wire.RipeHash
	copy(destination[:], to.Address.Ripe[:])

	data := &cipher.Bitmessage{
		FromStreamNumber:   from.Address.Stream,
		FromAddressVersion: from.Address.Version,
		Destination:        &destination,
		SigningKey:         pkd.VerificationKey,
		EncryptionKey:      pkd.EncryptionKey,
		Pow:                pkd.Pow,
		Content:            content,
	}

	message, err := cipher.SignAndEncryptMessage(time.Now().Add(expiry), from.Address.Stream, data, ack, from, to)
	if err != nil {
		return nil, nil, err
	}

	return message.Object(), pkd, nil
}

// GenerateObject generates the wire.MsgObject form of the message.
func (u *User) generateObject(m *email.Bmail, box *mailbox) (obj.Object, *obj.PubKeyData, error) {

	fromAddr, err := email.ToBm(m.From)
	if err != nil {
		return nil, nil, err
	}
	fromID := u.keys.Get(fromAddr)
	if fromID == nil {
		email.SMTPLog.Error("GenerateObject: no private id known ")
		return nil, nil, ErrMissingPrivateID
	}
	from := &(fromID.Private)

	// check for cached object.
	o := box.getObject(m)
	if o != nil {
		return o, from.ToPubKeyData(), nil
	}

	email.SMTPLog.Debug("GenerateObject: about to serialize bmsg from " + m.From + " to " + m.To)
	to, err := u.server.GetOrRequestPublicID(m.To)

	// If a pubkey request was sent, set the bmail's new state.
	if err != nil {
		if err == email.ErrGetPubKeySent {
			m.State.PubkeyRequestOutstanding = true
		}

		return nil, nil, err
	}

	// This is a brodcast.
	if to == Broadcast {
		return generateBroadcast(m.Content, from, u.expiration(wire.ObjectTypeBroadcast))
	}

	id := u.keys.Get(m.To)
	if id != nil {
		m.OfChannel = id.IsChan
		// We're sending to ourselves/chan so don't bother with ack.
		m.State.AckExpected = false
	} else if to.Behavior&identity.BehaviorAck == identity.BehaviorAck {
		// Set AckExpected if the flag is set in the public key.
		m.State.AckExpected = true
	}

	// Check for ack.
	if m.Ack == nil && m.State.AckExpected {
		return nil, nil, email.ErrAckMissing
	}

	return generateMessage(m.Content, m.Ack, from, to, u.expiration(wire.ObjectTypeMsg))
}

// GenerateAck creates an Ack message
func (u *User) generateAck(m *email.Bmail) (ack obj.Object, powData *pow.Data, err error) {
	// If this is a broadcast message, no ack is expected.
	if m.To == "broadcast@bm.agent" {
		return nil, nil, errors.New("No acks on broadcast messages.")
	}

	// Get our private key.
	fromAddr, err := email.ToBm(m.From)
	if err != nil {
		return nil, nil, err
	}
	from := u.keys.Get(fromAddr)

	addr := from.ToPublic().Address

	// Add the message, which is a random number.
	buf := new(bytes.Buffer)
	num := rand.Int31()
	err = binary.Write(buf, binary.LittleEndian, num)
	if err != nil {
		return nil, nil, err
	}

	// We don't save the message because it still needs POW done on it.
	return wire.NewMsgObject(
		wire.NewObjectHeader(0,
			time.Now().Add(u.expiration(wire.ObjectTypeMsg)),
			wire.ObjectTypeMsg,
			obj.MessageVersion,
			addr.Stream,
		), buf.Bytes()), &from.Data, nil
}
