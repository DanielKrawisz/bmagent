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

	"github.com/DanielKrawisz/bmagent/keymgr"
	"github.com/DanielKrawisz/bmagent/message/format"
	"github.com/DanielKrawisz/bmagent/powmgr"
	"github.com/DanielKrawisz/bmutil/identity"
	"github.com/DanielKrawisz/bmutil/pow"
	"github.com/DanielKrawisz/bmutil/wire"
	"github.com/jordwest/imap-server/mailstore"
	"github.com/jordwest/imap-server/types"
	"github.com/mailhog/data"
)

// User implements the mailstore.User interface and represents
// a collection of imap folders belonging to a single user.
type User struct {
	username string
	boxes    map[string]*mailbox
	keys     *keymgr.Manager
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
	}

	// The user is allowed to save in some mailboxes but not others.
	for _, mbox := range mboxes {
		var name = mbox.Name()
		var mb *mailbox
		var err error
		switch name {
		case DraftsFolderName:
			mb, err = NewDrafts(mbox, keys.Names())
		default:
			mb, err = NewMailbox(mbox, keys.Names())
		}
		if err != nil {
			return nil, err
		}
		u.boxes[name] = mb
	}

	return u, nil
}

// NewMailbox adds a new mailbox.
func (u *User) NewMailbox(name string) (Mailbox, error) {
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
func (u *User) DeliverFromBMNet(bm *Bitmessage) error {
	// Put message in the right folder.
	return u.boxes[InboxFolderName].AddNew(bm, types.FlagRecent)
}

// DeliverFromSMTP adds a message received via SMTP to the POW queue, if needed,
// and the outbox.
func (u *User) DeliverFromSMTP(smtp *data.Content) error {
	bmsg, err := NewBitmessageFromSMTP(smtp)
	if err != nil {
		smtpLog.Error("NewBitmessageFromSMTP gave error: ", err)
		return err
	}

	smtpLog.Debug("Bitmessage received by SMTP from " + bmsg.From + " to " + bmsg.To)

	// Check for command.
	if commandRegex.Match([]byte(bmsg.To)) {
		return errors.New("Commands not yet supported.")
	}

	outbox := u.boxes[OutboxFolderName]

	// Put message in outbox.
	err = outbox.AddNew(bmsg, types.FlagSeen)
	if err != nil {
		return nil
	}

	return u.trySend(bmsg)
}

// DeliverPublicKey takes a public key and attempts to match it with a message.
// If a matching message is found, the message is encoded to the wire format
// and sent to the pow queue.
func (u *User) DeliverPublicKey(bmaddr string, public *identity.Public) error {
	smtpLog.Debug("Deliver Public Key for address ", bmaddr)

	// Ensure that the address given is in the form of a bitmessage address.
	if !bitmessageRegex.Match([]byte(bmaddr)) {
		return errors.New("Bitmessage address required.")
	}

	outbox := u.boxes[OutboxFolderName]
	var bms []*Bitmessage

	// Go through all messages in the Outbox and get IDs of all the matches.
	err := outbox.mbox.ForEachMessage(0, 0, 2, func(id, _ uint64, msg []byte) error {
		bmsg, err := DecodeBitmessage(msg)
		if err != nil { // (Almost) impossible error.
			return err
		}

		// We have a match!
		if bmsg.state.PubkeyRequestOutstanding && strings.Contains(bmsg.To, bmaddr) {
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

func (u *User) Move(bmsg *Bitmessage, from, to string) error {
	fromBox := u.boxes[from]
	toBox := u.boxes[to]

	// Move message from old mailbox to the new one.
	err := fromBox.DeleteBitmessageByUID(bmsg.ImapData.UID)
	if err != nil {
		return err
	}

	bmsg.ImapData = nil
	return toBox.AddNew(bmsg, types.FlagSeen)
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
func (u *User) trySend(bmsg *Bitmessage) error {
	outbox := u.boxes[OutboxFolderName]

	// First we attempt to generate the wire.Object form of the message.
	// If we can't, then it is possible that we don't have the recipient's
	// pubkey. That is not an error state. If no object and no error is
	// returned, this is what has happened. If there is no object, then we
	// can't proceed futher so trySend completes.
	object, nonceTrials, extraBytes, err := bmsg.GenerateObject(u.server)
	if object == nil {
		smtpLog.Debug("trySend: could not generate message. Pubkey request sent? ", err == nil)
		if err == nil {
			bmsg.state.PubkeyRequestOutstanding = true

			err := outbox.saveBitmessage(bmsg)
			if err != nil {
				return err
			}
			return nil
		}
		return err
	}

	bmsg.state.PubkeyRequestOutstanding = false

	// sendPow takes a message in wire format with all required information
	// to send it over the network other than having proof-of-work run on it.
	// It generates the correct parameters for running the proof-of-work and
	// sends it to the proof-of-work queue with a function provided by the
	// user that says what to do with the completed message when the
	// proof-of-work is done.
	sendPow := func(object *wire.MsgObject, nonceTrials, extraBytes uint64, done func([]byte)) {
		encoded := wire.EncodeMessage(object)
		q := encoded[8:] // exclude the nonce

		target := pow.CalculateTarget(uint64(len(q)),
			uint64(object.ExpiresTime.Sub(time.Now()).Seconds()), nonceTrials, extraBytes)

		// Attempt to run pow on the message.
		u.server.RunPow(target, q, func(n powmgr.Nonce) {
			// Put the nonce bytes into the encoded form of the message.
			q = append(n.Bytes(), q...)
			done(q)
		})
	}

	// sendPreparedMessage is used after we have looked up private keys and
	// generated an ack message, if applicable.
	sendPreparedMessage := func() error {
		err := outbox.saveBitmessage(bmsg)
		if err != nil {
			return err
		}

		// Put the prepared object in the pow queue and send it off
		// through the network.
		sendPow(object.MsgObject(), nonceTrials, extraBytes, func(completed []byte) {
			err := func() error {
				// Save Bitmessage in outbox folder.
				err := u.boxes[OutboxFolderName].saveBitmessage(bmsg)
				if err != nil {
					return err
				}

				u.server.Send(completed)

				// Select new box for the message.
				var newBoxName string
				if bmsg.state.AckExpected {
					newBoxName = LimboFolderName
				} else {
					newBoxName = SentFolderName
				}

				bmsg.state.SendTries++
				bmsg.state.LastSend = time.Now()

				return u.Move(bmsg, OutboxFolderName, newBoxName)
			}()
			// We can't return the error any further because this function
			// isn't even run until long after trySend completes!
			if err != nil {
				smtpLog.Error("trySend: ", err)
			}
		})

		return nil
	}

	// We may desire to receive an ack with this message, which has not yet
	// been created. In that case, we need to do proof-of-work on the ack just
	// as on the entire message.
	if bmsg.Ack != nil && bmsg.state.AckExpected {
		ack, nonceTrialsAck, extraBytesAck, err := bmsg.generateAck(u.server)
		if err != nil {
			return err
		}

		err = outbox.saveBitmessage(bmsg)
		if err != nil {
			return err
		}

		sendPow(ack.MsgObject(), nonceTrialsAck, extraBytesAck, func(completed []byte) {
			err := func() error {
				// Add the ack to the message.
				bmsg.Ack = completed

				return sendPreparedMessage()
			}
			// Once again, we can't return the error any further because
			// trySend is over by the time this function is run.
			if err != nil {
				smtpLog.Error("trySend: ", err)
			}
		})
	} else {
		return sendPreparedMessage()
	}

	return nil
}

var ErrNoAckExpected = errors.New("No ack found.")

// DeliverAckReply takes a message ack and marks a message as having been
// received by the recipient.
func (u *User) DeliverAckReply(ack []byte) error {
	// Go through the Limbo folder and check if there is a message
	// that is expecting this ack.
	bmsg := u.boxes[LimboFolderName].ReceiveAck(ack)

	// Move the message to the sent folder.
	if bmsg != nil {
		return u.Move(bmsg, LimboFolderName, SentFolderName)
	}

	return ErrNoAckExpected
}

// Generate keys creates n new keys for the user and sends him a message
// about them.
func (u *User) GenerateKeys(n uint16) error {
	if n == 0 {
		return nil
	}

	inbox := u.boxes[InboxFolderName]
	if inbox == nil {
		return errors.New("Could not find inbox.")
	}

	// first generate the new keys.
	var i uint16
	keyList := ""
	for i = 0; i < n; i++ {
		addr := u.keys.NewHDIdentity(1, "").Address()

		keyList = fmt.Sprint(keyList, fmt.Sprintf("\t%s@bm.addr\n", addr))
	}

	message := fmt.Sprintf(newAddressesMsg, keyList)

	err := inbox.AddNew(&Bitmessage{
		From: "addresses@bm.agent",
		To:   "", /*send to all new addresses*/
		Content: &format.Encoding2{
			Subject: "New addresses generated.",
			Body:    message,
		},
	}, types.FlagRecent)
	if err != nil {
		return err
	}

	return nil
}
