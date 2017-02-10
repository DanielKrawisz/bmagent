// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package user

import (
	"errors"
	"strings"
	"time"

	"github.com/DanielKrawisz/bmagent/keymgr"
	"github.com/DanielKrawisz/bmagent/user/email"
	"github.com/DanielKrawisz/bmutil/identity"
	"github.com/DanielKrawisz/bmutil/pow"
	"github.com/DanielKrawisz/bmutil/wire"
	"github.com/DanielKrawisz/bmutil/wire/obj"
	"github.com/jordwest/imap-server/mailstore"
	"github.com/jordwest/imap-server/types"
	"github.com/mailhog/data"
)

var (
	// ErrUnrecognizedAck is returned if we have no record of having
	// sent such an ack.
	ErrUnrecognizedAck = errors.New("Unrecognized ack")

	// ErrNoAckExpected is returned if we somehow receive an ack for
	// a message for which none was expected.
	ErrNoAckExpected = errors.New("No ack expected")

	// ErrNoMessageFound is returned when no message is found.
	ErrNoMessageFound = errors.New("No message found")

	// ErrMissingPrivateID is returned when the private id could not be found.
	ErrMissingPrivateID = errors.New("Private id not found")
)

// User implements the mailstore.User interface and represents
// a collection of imap folders belonging to a single user.
type User struct {
	username string
	boxes    map[string]*mailbox
	keys     *keymgr.Manager
	acks     map[wire.ShaHash]uint64
	server   ServerOps
}

// NewUser creates a User object from the store.
func NewUser(username string, server ServerOps, keys *keymgr.Manager) (*User, error) {

	mboxes := server.Folders()

	if mboxes == nil {
		return nil, errors.New("Invalid user.")
	}

	u := &User{
		username: username,
		boxes:    make(map[string]*mailbox),
		server:   server,
		keys:     keys,
		acks:     make(map[wire.ShaHash]uint64),
	}

	// The user is allowed to save in some mailboxes but not others.
	for _, mbox := range mboxes {
		var name = mbox.Name()
		var mb *mailbox
		var err error
		switch name {
		case DraftsFolderName:
			mb, err = newDrafts(mbox, keys.Names())
		default:
			mb, err = newMailbox(mbox, keys.Names())
		}
		if err != nil {
			return nil, err
		}
		u.boxes[name] = mb
	}

	return u, nil
}

// NewMailbox adds a new mailbox.
func (u *User) NewMailbox(name string) (email.Mailbox, error) {
	return nil, errors.New("Not yet implemented.")
}

// Mailboxes returns all the mailboxes. It is part of the IMAPMailbox interface.
func (u *User) Mailboxes() []mailstore.Mailbox {
	mboxes := make([]mailstore.Mailbox, 0, len(u.boxes))
	for _, mbox := range u.boxes {
		mboxes = append(mboxes, mbox)
	}
	return mboxes
}

// MailboxByName returns a mailbox by its name. It is part of the IMAPMailbox
// interface.
func (u *User) MailboxByName(name string) (mailstore.Mailbox, error) {
	// Enforce the case insensitivity of Inbox.
	if strings.ToLower(name) == strings.ToLower(InboxFolderName) {
		name = InboxFolderName
	}

	mbox, ok := u.boxes[name]
	if !ok {
		return nil, errors.New("Not found")
	}
	return mbox, nil
}

// DeliverFromBMNet adds a message received from bmd into the appropriate
// folder.
func (u *User) DeliverFromBMNet(bm *email.Bmail) error {
	// Put message in the right folder.
	return u.boxes[InboxFolderName].AddNew(bm, types.FlagRecent)
}

// DeliverFromSMTP adds a message received via SMTP to the POW queue, if needed,
// and the outbox.
func (u *User) DeliverFromSMTP(smtp *data.Content) error {
	bmsg, err := email.NewBitmessageFromSMTP(smtp)
	if err != nil {
		email.SMTPLog.Error("NewBitmessageFromSMTP gave error: ", err)
		return err
	}

	email.SMTPLog.Debug("Bitmessage received by SMTP from " + bmsg.From + " to " + bmsg.To)

	// Check for command.
	if email.CommandRegex.Match([]byte(bmsg.To)) {
		return u.executeCommand(smtp.Headers["Subject"][0], smtp.Body)
	}

	outbox := u.boxes[OutboxFolderName]

	// Put message in outbox.
	err = outbox.AddNew(bmsg, types.FlagSeen)
	if err != nil {
		return nil
	}

	return u.trySend(&bmail{b: bmsg})
}

// DeliverPublicKey takes a public key and attempts to match it with a message.
// If a matching message is found, the message is encoded to the wire format
// and sent to the pow queue.
func (u *User) DeliverPublicKey(bmaddr string, public *identity.Public) error {
	email.SMTPLog.Debug("Deliver Public Key for address ", bmaddr)

	// Ensure that the address given is in the form of a bitmessage address.
	if !email.BitmessageRegex.Match([]byte(bmaddr)) {
		return errors.New("Bitmessage address required.")
	}

	outbox := u.boxes[OutboxFolderName]
	var bms []*bmail

	// Go through all messages in the Outbox and get IDs of all the matches.
	err := outbox.mbox.ForEachMessage(0, 0, 2, func(id, _ uint64, msg []byte) error {
		bmsg, err := decodeBitmessage(msg)
		if err != nil { // (Almost) impossible error.
			return err
		}

		// We have a match!
		if bmsg.b.State.PubkeyRequestOutstanding && strings.Contains(bmsg.b.To, bmaddr) {
			bms = append(bms, bmsg)
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, bmsg := range bms {
		if err := u.trySend(bmsg); err != nil {
			return err
		}
	}

	return nil
}

// Move finds a email.Bmail in one mailbox and moves it to another.
func (u *User) Move(bmsg *email.Bmail, from, to string) error {
	fromBox := u.boxes[from]
	toBox := u.boxes[to]

	uid := bmsg.ImapData.UID

	b := fromBox.bmsgByUID(uid)
	if b == nil {
		return nil
	}

	// Move message from old mailbox to the new one.
	err := fromBox.DeleteBitmessageByUID(uid)
	if err != nil {
		return err
	}

	b.b.ImapData = nil
	return toBox.addNew(b, types.FlagSeen)
}

// trySend takes a Bitmessage and does whatever needs to be done to it
// next in order to send it into the network. There are some steps in this
// process that take a bit of time, so they can't all be handled sequentially
// by one function. If we don't have the recipient's private key yet, we
// can't send the message and we have to send a pubkey request to him instead.
// On the other hand, messages might come in for which we already have the
// public key and then we can go on to the next step right away.
//
// The next step is proof-of-work, and that also takes time.
func (u *User) trySend(bmsg *bmail) error {
	email.SMTPLog.Debug("trySend called.")
	outbox := u.boxes[OutboxFolderName]

	// sendPow takes a message in wire format with all required information
	// to send it over the network other than having proof-of-work run on it.
	// It generates the correct parameters for running the proof-of-work and
	// sends it to the proof-of-work queue with a function provided by the
	// user that says what to do with the completed message when the
	// proof-of-work is done.
	sendPow := func(object obj.Object, powData *pow.Data, done func([]byte)) {
		encoded := wire.Encode(object)
		q := encoded[8:] // exclude the nonce

		target := pow.CalculateTarget(uint64(len(q)),
			uint64(object.Header().Expiration().Sub(time.Now()).Seconds()), *powData)

		// Attempt to run pow on the message.
		u.server.RunPow(target, q, func(n pow.Nonce) {
			// Put the nonce bytes into the encoded form of the message.
			q = append(n.Bytes(), q...)
			done(q)
		})
	}

	// sendPreparedMessage is used after we have looked up private keys and
	// generated an ack message, if applicable.
	sendPreparedMessage := func(object obj.Object, powData *pow.Data) error {
		email.SMTPLog.Debug("Generating pow for message.")
		err := outbox.saveBitmessage(bmsg)
		if err != nil {
			return err
		}

		// Put the prepared object in the pow queue and send it off
		// through the network.
		sendPow(object, powData, func(completed []byte) {
			err := func() error {
				// Save Bitmessage in outbox folder.
				err := u.boxes[OutboxFolderName].saveBitmessage(bmsg)
				if err != nil {
					return err
				}

				u.server.Send(completed)

				// Select new box for the message.
				var newBoxName string
				if bmsg.b.State.AckExpected {
					newBoxName = LimboFolderName
				} else {
					newBoxName = SentFolderName
				}

				bmsg.b.State.SendTries++
				bmsg.b.State.LastSend = time.Now()

				return u.Move(bmsg.b, OutboxFolderName, newBoxName)
			}()
			// We can't return the error any further because this function
			// isn't even run until long after trySend completes!
			if err != nil {
				email.SMTPLog.Error("trySend could not send message: ", err.Error())
			}
		})

		return nil
	}

	// First we attempt to generate the wire.Object form of the message.
	// If we can't, then it is possible that we don't have the recipient's
	// pubkey. That is not an error state. If no object and no error is
	// returned, this is what has happened. If there is no object, then we
	// can't proceed futher so trySend completes.
	object, data, err := bmsg.generateObject(u.server)

	if err != nil {
		if err == email.ErrGetPubKeySent {
			email.SMTPLog.Debug("trySend: Pubkey request sent.")

			// Try to save the message, as its state has changed.
			err := outbox.saveBitmessage(bmsg)
			if err != nil {
				return err
			}

			return nil
		} else if err == email.ErrAckMissing {
			email.SMTPLog.Debug("trySend: Generating ack.")
			ack, powData, err := bmsg.generateAck(u.server)
			if err != nil {
				return err
			}

			// Save the ack.
			u.acks[*obj.InventoryHash(ack)] = bmsg.b.ImapData.UID

			sendPow(ack, powData, func(completed []byte) {
				err := func() error {
					// Add the ack to the message.
					bmsg.b.Ack = completed

					// Attempt to generate object again. This time it
					// should work so we return every error.
					object, objData, err := bmsg.generateObject(u.server)
					if err != nil {
						return err
					}

					return sendPreparedMessage(object, objData.Pow)
				}()
				// Once again, we can't return the error any further because
				// trySend is over by the time this function is run.
				if err != nil {
					email.SMTPLog.Error("trySend: could not send pow ", err.Error())
				}
			})

			return nil
		} else {
			email.SMTPLog.Debug("trySend: could not generate message.", err.Error())
			return err
		}
	}

	// If the object was generated successufully, do POW and send it.
	return sendPreparedMessage(object, data.Pow)
}

// DeliverAckReply takes a message ack and marks a message as having been
// received by the recipient.
func (u *User) DeliverAckReply(hash *wire.ShaHash) error {
	uid, ok := u.acks[*hash]
	if !ok {
		return ErrUnrecognizedAck
	}

	bmsg := u.boxes[LimboFolderName].bmsgByUID(uid)

	// Move the message to the sent folder.
	if bmsg != nil {
		if !bmsg.b.State.AckExpected {
			return ErrNoAckExpected
		}
		bmsg.b.State.AckReceived = true
		return u.Move(bmsg.b, LimboFolderName, SentFolderName)
	}

	return ErrNoMessageFound
}
