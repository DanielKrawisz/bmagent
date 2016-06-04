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
	"github.com/DanielKrawisz/bmagent/keymgr"
	"github.com/DanielKrawisz/bmagent/message/format"
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
		username : username,
		boxes:  make(map[string]*mailbox),
		server: server,
		keys: keys, 
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
func (u *User) DeliverFromSMTP(bmsg *Bitmessage) error {
	smtpLog.Debug("Bitmessage received by SMTP from " + bmsg.From + " to " + bmsg.To)
	
	// Check for command. 
	if commandRegex.Match([]byte(bmsg.To)) {
		return errors.New("Commands not yet supported.")
	} 
	
	outbox := u.boxes[OutboxFolderName]

	// Put message in outbox.
	err := outbox.AddNew(bmsg, types.FlagSeen)
	if err != nil {
		return nil
	}

	// Attempt to run pow on the message and send it off on the network.
	// This will only happen if the pubkey can be found. An error is only
	// returned if the message could not be generated and the pubkey request
	// could not be sent.
	err = bmsg.SubmitPow(u.server)
	if err != nil {
		smtpLog.Error("Unable to submit for proof-of-work: ", err)
		return err
	}
	
	// Save Bitmessage with pow index.
	err = outbox.saveBitmessage(bmsg)
	if err != nil {
		return err
	}
	return nil
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
	var ids []uint64

	// Go through all messages in the Outbox and get IDs of all the matches.
	err := outbox.mbox.ForEachMessage(0, 0, 2, func(id, _ uint64, msg []byte) error {
		bmsg, err := DecodeBitmessage(msg)
		if err != nil { // (Almost) impossible error.
			return err
		}
		
		// We have a match!
		if bmsg.state.PubkeyRequestOutstanding && strings.Contains(bmsg.To, bmaddr) {
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil {
		return err
	}

	outbox.Lock()
	defer outbox.Unlock()
	
	for _, id := range ids {
		bmsg := outbox.BitmessageByUID(id)
		bmsg.state.PubkeyRequestOutstanding = false

		if bmsg.state.AckExpected {
			// TODO generate the ack and send it to the pow queue.
		}

		// Add the message to the pow queue.
		err := bmsg.SubmitPow(u.server)
		if err != nil {
			return errors.New("Unable to add message to pow queue.")
		}

		// Save Bitmessage with pow index.
		err = outbox.saveBitmessage(bmsg)
		if err != nil {
			return err
		}
	}

	return nil
}

// DeliverPow delivers an object that has had pow done on it.
func (u *User) DeliverPow(index uint64, obj *wire.MsgObject) error {
	outbox := u.boxes[OutboxFolderName]

	var bmsg *Bitmessage
	var idMsg uint64
	
	// The error returned by the inner func which signifies that 
	// a message was found successfully. This error is used to represent
	// a success case for the query. 
	var errMessageFound = errors.New("Message found.")

	// Go through all messages in the Outbox and get IDs of all the matches.
	err := outbox.mbox.ForEachMessage(0, 0, 2, func(id, _ uint64, msg []byte) error {
		var dbErr error
		bmsg, dbErr = DecodeBitmessage(msg)
		if dbErr != nil { // (Almost) impossible error.
			return dbErr
		}
		// We have a match!
		if bmsg.state.PowIndex == index {
			idMsg = id
			return errMessageFound
		}
		return nil
	})
	if err == nil {
		return fmt.Errorf("Unable to find message in outbox with POW index %d",
			index)
	}
	if err != errMessageFound {
		return err
	}

	smtpLog.Trace("pow delivered for messege from " + bmsg.From + " to " + bmsg.To)

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
	// TODO
}

// DeliverAckReply takes a message ack and marks a message as having been
// received by the recipient.
func (u *User) DeliverAckReply() {
	// TODO
}

// Generate keys creates n new keys for the user and sends him a message
// about them. 
func (u *User) GenerateKeys(n uint16) error {
	if n == 0 {
		return nil
	}
	
	inbox := u.boxes[InboxFolderName];
	if inbox == nil {
		return errors.New("Could not find inbox.");
	}
	
	// first generate the new keys. 
	var i uint16;
	keyList := ""
	for i = 0; i < n; i ++ {
		addr := u.keys.NewHDIdentity(1, "").Address()
		
		keyList = fmt.Sprint(keyList, fmt.Sprintf("\t%s@bm.addr\n", addr))
	}
	
	message := fmt.Sprintf(newAddressesMsg, keyList)
	
	err := inbox.AddNew(&Bitmessage{
		From: "addresses@bm.agent", 
		To: "" /*send to all new addresses*/,
		Message: &format.Encoding2{
			Subject: "New addresses generated.",
			Body: message, 
		}, 
	}, types.FlagRecent)
	if err != nil {
		return err
	}
	
	return nil
}
