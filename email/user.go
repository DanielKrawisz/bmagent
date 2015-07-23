package email

import (
	"errors"
	"strings"
	"time"

	"github.com/jordwest/imap-server/mailstore"
	"github.com/jordwest/imap-server/types"
	"github.com/monetas/bmclient/keymgr"
	"github.com/monetas/bmclient/store"
	"github.com/monetas/bmutil/identity"
	"github.com/monetas/bmutil/wire"
)

// User implements the email.IMAPAccount interface and represents
// a collection of imap folders belonging to a single user.
type User struct {
	boxes map[string]*Mailbox

	addressBook keymgr.AddressBook

	powQueue *store.PowQueue
}

// NewUser creates a User object from the store.
func NewUser(s *store.Store, book keymgr.AddressBook) (*User, error) {
	u := &User{
		boxes:       make(map[string]*Mailbox),
		addressBook: book,
		powQueue:    s.PowQueue,
	}

	mboxes := s.Mailboxes()
	// The user is allowed to save in some mailboxes but not others.
	for _, mbox := range mboxes {
		mb, err := NewMailbox(mbox)
		if err != nil {
			return nil, err
		}
		u.boxes[mb.Name()] = mb
	}

	// Add any mailboxes that are missing.
	for _, boxName := range []string{InboxFolderName, SentFolderName, OutboxFolderName,
		DraftsFolderName, TrashFolderName, LimboFolderName, CommandFolderName} {
		if _, ok := u.boxes[boxName]; !ok {
			newbox, err := s.NewMailbox(boxName)
			if err != nil {
				return nil, err
			}
			mb, err := NewMailbox(newbox)
			if err != nil {
				return nil, err
			}
			u.boxes[boxName] = mb
		}
	}

	u.powQueue = s.PowQueue

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

// DeliverFromBMNet
func (u *User) DeliverFromBMNet(bm *Bitmessage) error {
	// Put message in the right folder.
	box, ok := u.boxes[InboxFolderName]
	if !ok {
		return errors.New("Cannot find inbox.")
	}
	return box.AddNew(bm, types.FlagRecent)
}

// DeliverFromSMTP
func (u *User) DeliverFromSMTP(bm *Bitmessage) error {
	smtpLog.Trace("Bitmessage received from SMTP")

	// Attempt to run pow on the message and send it off on the network.
	// This will only happen if the pubkey can be found. An error is only
	// returned if the message could not be generated and the pubkey request
	// could not be sent.
	_, err := bm.SubmitPow(u.powQueue, u.addressBook)
	if err != nil {
		smtpLog.Error("Unable to submit proof-of-work.")
		return err
	}

	// Put message in the right folder.
	box, ok := u.boxes[OutboxFolderName]
	if !ok {
		return errors.New("Cannot find outbox.")
	}
	return box.AddNew(bm, types.FlagRecent&types.FlagSeen)
}

// DeliverPublicKey takes a public key and attempts to match it with a message.
// If a matching message is found, the message is encoded to the wire format
// and sent to the pow queue.
func (u *User) DeliverPublicKey(address string, public *identity.Public) error {
	// Add the address to the address book.
	addr, err := public.Address.Encode()
	if err != nil {
		return err
	}
	u.addressBook.AddPublicIdentity(addr, public)

	outbox, ok := u.boxes[OutboxFolderName]
	if !ok {
		return errors.New("Outbox not found")
	}

	msgIndex := outbox.getPKRequestIndex(address)
	if msgIndex == 0 {
		return errors.New("Message not found")
	}

	bmsg := outbox.BitmessageByUID(msgIndex)
	if bmsg == nil {
		return errors.New("Message not found")
	}

	bmsg.state.PubkeyRequested = false
	if bmsg.state.SendTries > 0 {
		return errors.New("Message already sent")
	}

	if bmsg.state.AckExpected {
		// TODO generate the ack and send it to the pow queue.
	}

	// Add the message to the pow queue.
	_, err = bmsg.SubmitPow(u.powQueue, u.addressBook)
	if err != nil {
		smtpLog.Error("Unable to send message to pow queue.")
	}

	smtpLog.Trace("DeliverPublicKey: message submitted to pow queue.")

	return outbox.SaveBitmessage(bmsg)
}

// DeliverPow delivers an object that has had pow done on it.
func (u *User) DeliverPow(index uint64, obj *wire.MsgObject) error {
	var fromOutbox, toSent bool
	var oldbox *Mailbox
	oldbox, fromOutbox = u.boxes[OutboxFolderName]
	if !fromOutbox {
		var ok bool
		oldbox, ok = u.boxes[LimboFolderName]
		if !ok {
			return errors.New("Outbox not found.")
		}
	}

	msgIndex := oldbox.getPowQueueIndex(index)
	if msgIndex == 0 {
		return errors.New("Message not found")
	}

	bmsg := oldbox.BitmessageByUID(msgIndex)
	if bmsg == nil {
		return errors.New("Message not found")
	}

	// Select new box for the message.
	var newBoxName string
	if bmsg.state.AckExpected {
		newBoxName = LimboFolderName
	} else {
		newBoxName = SentFolderName
		toSent = true
	}
	newBox, ok := u.boxes[newBoxName]
	if !ok {
		return errors.New("Folder not found")
	}

	bmsg.state.PowIndex = 0
	bmsg.state.SendTries++
	bmsg.state.LastSend = time.Now()

	smtpLog.Trace("Message updated.")

	if fromOutbox || toSent {
		oldbox.DeleteBitmessageByUID(msgIndex)
		bmsg.ImapData = nil
		return newBox.AddNew(bmsg, types.FlagRecent)
	}

	return oldbox.SaveBitmessage(bmsg)
}

// DeliverPowAck delivers an ack message that was generated by the pow queue.
func (u *User) DeliverPowAck() {

}

// DeliverAck takes a message ack and marks a message as having been received
// by the recipient.
func (u *User) DeliverAckReply() {

}
