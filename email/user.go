// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jordwest/imap-server/mailstore"
	"github.com/jordwest/imap-server/types"
	"github.com/DanielKrawisz/bmutil/identity"
	"github.com/DanielKrawisz/bmutil/wire"
)

// User implements the mailstore.User interface and represents
// a collection of imap folders belonging to a single user.
type User struct {
	boxes  map[string]*Mailbox
	server ServerOps
}

// NewUser creates a User object from the store.
func NewUser(server ServerOps) (*User, error) {
	u := &User{
		boxes:  make(map[string]*Mailbox),
		server: server,
	}

	mboxes := server.Store().Mailboxes()
	// The user is allowed to save in some mailboxes but not others.
	for _, mbox := range mboxes {
		var name = mbox.Name()
		var mb *Mailbox
		var err error
		switch name {
			case DraftsFolderName:
			mb, err = NewDrafts(mbox)
			default:
			mb, err = NewMailbox(mbox)
		}
		if err != nil {
			return nil, err
		}
		u.boxes[name] = mb
	}

	return u, nil
}

// NewMailbox adds a new mailbox.
func (u *User) NewMailbox(name string) (*Mailbox, error) {
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
		return u.boxes[InboxFolderName], nil
	}

	mbox, ok := u.boxes[name]
	if !ok {
		return nil, errors.New("Not found")
	}
	return mbox, nil
}

// DeliverFromBMNet adds a message received from bmd into the appropriate
// folder.
func (u *User) DeliverFromBMNet(bm *Bitmessage) error {
	// Put message in the right folder.
	return u.boxes[InboxFolderName].AddNew(bm, types.FlagRecent)
}

// DeliverFromSMTP adds a message received via SMTP to the POW queue, if needed,
// and the outbox.
func (u *User) DeliverFromSMTP(bm *Bitmessage) error {
	smtpLog.Trace("Bitmessage received from SMTP")

	// Attempt to run pow on the message and send it off on the network.
	// This will only happen if the pubkey can be found. An error is only
	// returned if the message could not be generated and the pubkey request
	// could not be sent.
	_, err := bm.SubmitPow(u.server.PowQueue(), u.server)
	if err != nil {
		smtpLog.Error("Unable to submit for proof-of-work: ", err)
		return err
	}

	// Put message in the right folder.
	return u.boxes[OutboxFolderName].AddNew(bm, types.FlagSeen)
}

// DeliverPublicKey takes a public key and attempts to match it with a message.
// If a matching message is found, the message is encoded to the wire format
// and sent to the pow queue.
func (u *User) DeliverPublicKey(address string, public *identity.Public) error {
	outbox := u.boxes[OutboxFolderName]
	var ids []uint64

	// Go through all messages in the Outbox and get IDs of all the matches.
	err := outbox.mbox.ForEachMessage(0, 0, 2, func(id, _ uint64, msg []byte) error {
		bmsg, err := DecodeBitmessage(msg)
		if err != nil { // (Almost) impossible error.
			return err
		}
		// We have a match!
		if bmsg.state.PubkeyRequested == true && bmsg.To == address {
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil {
		return err
	}

	outbox.Lock()
	for _, id := range ids {
		bmsg := outbox.BitmessageByUID(id)
		bmsg.state.PubkeyRequested = false
		err = outbox.SaveBitmessage(bmsg)
		if err != nil {
			return err
		}

		if bmsg.state.AckExpected {
			// TODO generate the ack and send it to the pow queue.
		}

		// Add the message to the pow queue.
		_, err := bmsg.SubmitPow(u.server.PowQueue(), u.server)
		if err != nil {
			return errors.New("Unable to add message to pow queue.")
		}

		// Save Bitmessage with pow index.
		err = outbox.SaveBitmessage(bmsg)
		if err != nil {
			return err
		}
	}
	outbox.Unlock()

	return nil
}

// DeliverPow delivers an object that has had pow done on it.
func (u *User) DeliverPow(index uint64, obj *wire.MsgObject) error {
	outbox := u.boxes[OutboxFolderName]

	var idMsg uint64

	// Go through all messages in the Outbox and get IDs of all the matches.
	err := outbox.mbox.ForEachMessage(0, 0, 2, func(id, _ uint64, msg []byte) error {
		bmsg, err := DecodeBitmessage(msg)
		if err != nil { // (Almost) impossible error.
			return err
		}
		// We have a match!
		if bmsg.state.PowIndex == index {
			idMsg = id
			return errors.New("Message found.")
		}
		return nil
	})
	if err == nil {
		return fmt.Errorf("Unable to find message in outbox with POW index %d",
			index)
	}

	bmsg := outbox.BitmessageByUID(idMsg)

	// Select new box for the message.
	var newBoxName string
	if bmsg.state.AckExpected {
		newBoxName = LimboFolderName
	} else {
		newBoxName = SentFolderName
	}
	newBox := u.boxes[newBoxName]

	bmsg.state.PowIndex = 0
	bmsg.state.SendTries++
	bmsg.state.LastSend = time.Now()

	// Move message from Outbox to the new mailbox.
	err = outbox.DeleteBitmessageByUID(idMsg)
	if err != nil {
		return err
	}

	bmsg.ImapData = nil
	return newBox.AddNew(bmsg, types.FlagSeen)
}

// DeliverPowAck delivers an ack message that was generated by the pow queue.
func (u *User) DeliverPowAck() {

}

// DeliverAckReply takes a message ack and marks a message as having been
// received by the recipient.
func (u *User) DeliverAckReply() {

}
