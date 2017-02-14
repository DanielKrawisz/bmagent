// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package user

import (
	"errors"
	"strings"
	"time"

	"github.com/DanielKrawisz/bmagent/keymgr/keys"
	"github.com/DanielKrawisz/bmagent/user/email"
	"github.com/DanielKrawisz/bmutil/identity"
	"github.com/DanielKrawisz/bmutil/wire"
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

// ObjectExpiration returns the time duration after which an object of the
// given type will expire on the network. It's used for POW calculations.
type ObjectExpiration func(wire.ObjectType) time.Duration

// User implements the mailstore.User interface and represents
// a collection of imap folders belonging to a single user.
type User struct {
	username   string
	boxes      map[string]*mailbox
	acks       map[wire.ShaHash]uint64
	expiration ObjectExpiration
	keys       keys.Manager
	server     ServerOps
}

// NewUser creates a User object from the store.
func NewUser(username string, privateIds keys.Manager, expiration ObjectExpiration, server ServerOps) (*User, error) {

	mboxes := server.Folders()

	if mboxes == nil {
		return nil, errors.New("Invalid user.")
	}

	u := &User{
		username: username,
		boxes:    make(map[string]*mailbox),
		server:   server,
		keys:     privateIds,
		acks:     make(map[wire.ShaHash]uint64),
	}

	// The user is allowed to save in some mailboxes but not others.
	for _, mbox := range mboxes {
		var name = mbox.Name()
		var mb *mailbox
		var err error
		switch name {
		case DraftsFolderName:
			mb, err = newDrafts(mbox, u.keys.Names())
		default:
			mb, err = newMailbox(mbox, u.keys.Names())
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

	return u.process(bmsg)
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
	var bms []*email.Bmail

	// Go through all messages in the Outbox and get IDs of all the matches.
	err := outbox.mbox.ForEachMessage(0, 0, 2, func(id, _ uint64, msg []byte) error {
		bmsg, _, err := decodeBitmessage(msg)
		if err != nil { // (Almost) impossible error.
			return err
		}

		// We have a match!
		if bmsg.State.PubkeyRequestOutstanding && strings.Contains(bmsg.To, bmaddr) {
			bms = append(bms, bmsg)
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, bmsg := range bms {
		if err := u.process(bmsg); err != nil {
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

	b.ImapData = nil
	return toBox.addNew(b, types.FlagSeen)
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
		if !bmsg.State.AckExpected {
			return ErrNoAckExpected
		}
		bmsg.State.AckReceived = true
		return u.Move(bmsg, LimboFolderName, SentFolderName)
	}

	return ErrNoMessageFound
}
