// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package message

import (
	"bytes"
	"container/list"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jordwest/imap-server/mailstore"
	"github.com/jordwest/imap-server/types"
	"github.com/mailhog/data"
	"github.com/monetas/bmclient/email"
)

const (
	encoding2 = 2
)

// GetSequenceNumber gets a sequence number of a message by its uid.
// If there is no message with the right uid, then it returns 0.
func GetSequenceNumber(uids []uint64, uid uint64) uint32 {
	// If the slice is empty.
	if len(uids) == 0 {
		return 0
	}

	// If there is only one message in the box.
	if len(uids) == 1 {
		if uids[0] == uid {
			return 1
		}
		return 0
	}

	var minIndex uint32
	maxIndex := uint32(len(uids) - 1)

	minUID := uids[minIndex]
	maxUID := uids[maxIndex]

	for {
		ratio := (float64(uid - minUID)) / (float64(maxUID - minUID))
		checkIndex := uint32(math.Floor(float64(minIndex) + ratio*float64(minIndex-maxIndex)))

		newUID := uids[checkIndex]

		if newUID == uid {
			return checkIndex + 1
		}

		if newUID > uid {
			maxUID = newUID
		} else {
			// Add 1 because we use Floor function earlier.
			minUID = newUID + 1
		}

		if minIndex == maxIndex {
			return 0
		}
	}
}

type Mailbox interface {
	InsertMessage(msg []byte, id, suffix uint64) (uint64, error)
	GetMessage(id uint64) (uint64, []byte, error)
	ForEachMessage(lowID, highID, suffix uint64,
		f func(id, suffix uint64, msg []byte) error) error
	DeleteMessage(id uint64) error
	GetName() string
	GetNextID() (uint64, error)
	GetLastID() (uint64, error)
	GetLastIDBySuffix(suffix uint64) (uint64, error)
}

// Folder implements a bitmessage folder that is compatible with imap.
// It implements the email.ImapFolder interface.
type Folder struct {
	mailbox   Mailbox
	uids      []uint64
	numRecent uint32
	numUnseen uint32
}

func (box *Folder) decodeBitmessageForImap(uid uint64, seqno uint32, msg []byte) *Bitmessage {
	b, _ := DecodeBitmessage(msg)
	if b == nil {
		return nil
	}
	b.ImapData.UID = uid
	b.ImapData.SequenceNumber = seqno
	b.ImapData.Folder = box
	return b
}

// Name returns the name of the mailbox.
// This is part of the email.ImapFolder interface.
func (box *Folder) Name() string {
	return box.mailbox.GetName()
}

// NextUID returns the unique identifier that will LIKELY be assigned
// to the next mail that is added to this mailbox.
// This is part of the email.ImapFolder interface.
func (box *Folder) NextUID() uint32 {
	n, _ := box.mailbox.GetNextID()
	return uint32(n)
}

// LastUID assigns the UID of the very last message in the mailbox
// If the mailbox is empty, this should return the next expected UID.
// This is part of the email.ImapFolder interface.
func (box *Folder) LastUID() uint32 {
	last, err := box.mailbox.GetLastIDBySuffix(encoding2)
	if err != nil {
		return box.NextUID()
	}
	return uint32(last)
}

// Recent returns the number of recent messages in the mailbox
// This is part of the email.ImapFolder interface.
func (box *Folder) Recent() uint32 {
	return box.numRecent
}

// Messages returns the number of messages in the mailbox
// This is part of the email.ImapFolder interface.
func (box *Folder) Messages() uint32 {
	return uint32(len(box.uids))
}

// Unseen returns the number of messages that do not have the Unseen flag set yet
// This is part of the email.ImapFolder interface.
func (box *Folder) Unseen() uint32 {
	return box.numUnseen
}

// BitmessageBySequenceNumber gets a message by its sequence number
func (box *Folder) BitmessageBySequenceNumber(seqno uint32) *Bitmessage {
	if seqno < 1 || seqno > box.Messages() {
		return nil
	}
	uid := box.uids[seqno-1]
	suffix, msg, err := box.mailbox.GetMessage(uid)
	if err != nil || suffix != 2 {
		return nil
	}

	return box.decodeBitmessageForImap(uid, seqno, msg)
}

// MessageBySequenceNumber gets a message by its sequence number
// It is a part of the mail.SMTPFolder interface.
func (box *Folder) MessageBySequenceNumber(seqno uint32) mailstore.Message {
	letter := box.BitmessageBySequenceNumber(seqno)
	if letter == nil {
		return nil
	}
	email, _ := letter.ToEmail()
	return email
}

// BitmessageByUID returns a message by its uid.
func (box *Folder) BitmessageByUID(uidno uint64) *Bitmessage {
	fmt.Println("bitmessage by uid:", uidno)
	suffix, msg, err := box.mailbox.GetMessage(uint64(uidno))
	if err != nil || suffix != 2 {
		return nil
	}
	seqno := GetSequenceNumber(box.uids, uint64(uidno))
	fmt.Println("bitmessage by uid:", seqno)

	return box.decodeBitmessageForImap(uidno, seqno, msg)
}

// MessageByUID gets a message by its uid number
// It is a part of the mail.SMTPFolder interface.
func (box *Folder) MessageByUID(uidno uint32) mailstore.Message {
	letter := box.BitmessageByUID(uint64(uidno))
	if letter == nil {
		return nil
	}
	email, _ := letter.ToEmail()
	return email
}

// LastBitmessage returns the last Bitmessage in the mailbox.
func (box *Folder) LastBitmessage() *Bitmessage {
	if len(box.uids) == 0 {
		return nil
	}

	uid := box.uids[len(box.uids)-1]
	_, msg, _ := box.mailbox.GetMessage(uid)
	return box.decodeBitmessageForImap(uid, uint32(len(box.uids)), msg)
}

// getRange returns a sequence of bitmessages from the mailbox in a range from
// startUID to endUID. It does not check whether the given sequence numbers make sense.
func (box *Folder) getRange(startUID, endUID uint64, startSequence, endSequence uint32) []*Bitmessage {
	fmt.Println("bitmessage set by uid:", startSequence)
	bitmessages := make([]*Bitmessage, endSequence-startSequence+1)

	var i uint32

	gen := func(id, suffix uint64, msg []byte) error {
		if suffix != 2 {
			return nil
		}
		if msg == nil {
			panic("gen... Should not be nil!!!!")
		}
		bm := box.decodeBitmessageForImap(id, startSequence+i, msg)
		if bm == nil {
			fmt.Println("Message is totally nil!!!")
			return errors.New("Message should not be nil")
		}
		bitmessages[i] = bm
		i++
		return nil
	}

	if box.mailbox.ForEachMessage(uint64(startUID), uint64(endUID), 2, gen) != nil {
		return nil
	}
	return bitmessages[:i]
}

// getSince returns a sequence of bitmessages from the mailbox which includes
// all greater than or equal to a given uid number. It does not check whether
// the given sequence number makes sense.
func (box *Folder) getSince(startUID uint64, startSequence uint32) []*Bitmessage {
	return box.getRange(startUID, 0, startSequence, box.Messages())
}

// BitmessagesByUIDRange returns the last Bitmessage in the mailbox.
func (box *Folder) BitmessagesByUIDRange(start, end uint64) []*Bitmessage {
	startSequence := GetSequenceNumber(box.uids, start)
	if startSequence == 0 {
		return nil
	}
	endSequence := GetSequenceNumber(box.uids, end)
	if endSequence == 0 || endSequence < startSequence {
		return nil
	}
	return box.getRange(start, end, startSequence, endSequence)
}

// BitmessagesSinceUID returns the last Bitmessage in the mailbox.
func (box *Folder) BitmessagesSinceUID(start uint64) []*Bitmessage {
	fmt.Println("bitmessage set by uid:", start)
	startSequence := GetSequenceNumber(box.uids, start)
	fmt.Println("bitmessage set by uid:", startSequence)
	if startSequence == 0 {
		return []*Bitmessage{}
	}
	return box.getSince(start, startSequence)
}

// BitmessagesBySequenceRange returns a set of Bitmessages in a range between two sequence numbers inclusive.
func (box *Folder) BitmessagesBySequenceRange(start, end uint32) []*Bitmessage {
	if start < 1 || start > box.Messages() || end < 1 || end > box.Messages() || end < start {
		return nil
	}
	startUID := box.uids[start]
	endUID := box.uids[end]
	return box.getRange(startUID, endUID, start, end)
}

// BitmessagesSinceSequenceNumber returns the set of Bitmessages since and including a given uid value.
func (box *Folder) BitmessagesSinceSequenceNumber(start uint32) []*Bitmessage {
	if start < 1 || start > box.Messages() {
		return nil
	}
	startUID := box.uids[start]
	return box.getSince(startUID, start)
}

// BitmessageSetByUID gets messages belonging to a set of ranges of UIDs
func (box *Folder) BitmessageSetByUID(set types.SequenceSet) []*Bitmessage {
	var msgs []*Bitmessage
	fmt.Println("bitmessage set by uid:", set)

	// If the mailbox is empty, return empty array
	if box.Messages() == 0 {
		return msgs
	}

	for _, msgRange := range set {
		// If Min is "*", meaning the last UID in the mailbox, Max should
		// always be Nil
		if msgRange.Min.Last() {
			// Return the last message in the mailbox
			msgs = append(msgs, box.LastBitmessage())
			continue
		}

		start, err := msgRange.Min.Value()
		if err != nil {
			return msgs
		}

		// If no Max is specified, then return only the min value.
		if msgRange.Max.Nil() {
			// Fetch specific message by sequence number
			msgs = append(msgs, box.BitmessageByUID(uint64(start)))
			if err != nil {
				return msgs
			}
			continue
		}

		var end uint32
		if msgRange.Max.Last() {
			since := box.BitmessagesSinceUID(uint64(start))
			if since == nil {
				panic("since should not be nil!")
			}
			msgs = append(msgs, since...)
		} else {
			end, err = msgRange.Max.Value()
			if err != nil {
				return msgs
			}
			msgs = append(msgs, box.BitmessagesByUIDRange(uint64(start), uint64(end))...)
		}
	}

	fmt.Println("bitmessage set by uid; len = ", len(msgs), "; msgs = ", msgs)
	return msgs
}

// BitmessageSetBySequenceNumber gets messages belonging to a set of ranges of sequence numbers
func (box *Folder) BitmessageSetBySequenceNumber(set types.SequenceSet) []*Bitmessage {
	var msgs []*Bitmessage

	// If the mailbox is empty, return empty array
	if box.Messages() == 0 {
		return msgs
	}

	for _, msgRange := range set {
		// If Min is "*", meaning the last UID in the mailbox, Max should
		// always be Nil
		if msgRange.Min.Last() {
			// Return the last message in the mailbox
			msgs = append(msgs, box.LastBitmessage())
			continue
		}

		startIndex, err := msgRange.Min.Value()
		if err != nil {
			return msgs
		}
		if startIndex < 1 || startIndex > box.Messages() {
			return msgs
		}
		start := uint32(box.uids[startIndex-1])

		// If no Max is specified, then return only the min value.
		if msgRange.Max.Nil() {
			// Fetch specific message by sequence number
			msgs = append(msgs, box.BitmessageBySequenceNumber(start))
			if err != nil {
				return msgs
			}
			continue
		}

		var end uint32
		if msgRange.Max.Last() {
			msgs = append(msgs, box.BitmessagesSinceSequenceNumber(start)...)
		} else {
			end, err = msgRange.Max.Value()
			if err != nil {
				return msgs
			}
			msgs = append(msgs, box.BitmessagesBySequenceRange(start, end)...)
		}
	}

	return msgs
}

func (box *Folder) updateUIDs(uid uint64) {
	box.uids = append(box.uids, uid)

	box.numRecent++
	box.numUnseen++
}

// AddNew adds a new bitmessage to the storebox.
func (box *Folder) AddNew(bmsg *Bitmessage, flags types.Flags) (*Bitmessage, error) {
	encoding := bmsg.Payload.Encoding()
	if encoding != 2 {
		return nil, errors.New("Invalid encoding")
	}

	imapData := &ImapData{
		SequenceNumber: uint32(len(box.uids)),
		Flags:          flags,
		DateReceived:   time.Now(),
		Folder:         box,
	}

	bmsg.ImapData = imapData

	msg, err := bmsg.Serialize()
	if err != nil {
		return nil, err
	}

	uid, err := box.mailbox.InsertMessage(msg, encoding, bmsg.Payload.Encoding())
	if err != nil {
		return nil, err
	}

	imapData.UID = uid
	box.updateUIDs(uid)

	return bmsg, nil
}

// MessageSetByUID returns the slice of messages belonging to a set of ranges of UIDs
// It is a part of the mail.SMTPFolder interface.
func (box *Folder) MessageSetByUID(set types.SequenceSet) []mailstore.Message {
	msgs := box.BitmessageSetByUID(set)
	email := make([]mailstore.Message, len(msgs))
	for _, msg := range msgs {
		if msg == nil {
			panic("Should not be nil!")
		}
		email[msg.ImapData.SequenceNumber-1], _ = msg.ToEmail()
	}
	return email
}

// MessageSetBySequenceNumber returns the slice of messages belonging to a set
// of ranges of sequence numbers
// It is a part of the mail.SMTPFolder interface.
func (box *Folder) MessageSetBySequenceNumber(set types.SequenceSet) []mailstore.Message {
	msgs := box.BitmessageSetBySequenceNumber(set)
	email := make([]mailstore.Message, len(msgs))
	for i, msg := range msgs {
		email[i], _ = msg.ToEmail()
	}
	return email
}

// Save saves the given bitmessage entry in the folder.
func (box *Folder) SaveBitmessage(msg *Bitmessage) error {
	if msg.ImapData.UID != 0 { // The message already exists and needs to be replaced.
		// Check that the uid, date, and sequence number are consistent with one another.
		previous := box.BitmessageByUID(msg.ImapData.UID)
		if previous == nil {
			return errors.New("Invalid sequence number")
		}
		if previous.ImapData.UID != msg.ImapData.UID {
			return errors.New("Invalid uid")
		}
		if previous.ImapData.DateReceived != msg.ImapData.DateReceived {
			// TODO check if this is true.
			return errors.New("Cannot change date")
		}

		// Delete the old message from the database.
		err := box.mailbox.DeleteMessage(uint64(msg.ImapData.UID))
		if err != nil {
			return err
		}
	}

	// Generate the new version of the message.
	encode, err := msg.Serialize()
	if err != nil {
		return err
	}

	// insert the new version of the message.
	newUID, err := box.mailbox.InsertMessage(encode, msg.ImapData.UID, encoding2)
	if err != nil {
		return err
	}

	if msg.ImapData.UID == 0 {
		msg.ImapData.UID = newUID
		box.updateUIDs(newUID)
	}
	return nil
}

// Save saves an imap email to its folder.
// It is a part of the mail.SMTPFolder interface.
func (box *Folder) Save(email *email.ImapEmail) error {
	bm, err := NewBitmessageFromSMTP(email.Content)
	if err != nil {
		return err
	}

	bm.ImapData = &ImapData{
		UID:            email.ImapUID,
		SequenceNumber: email.ImapSequenceNumber,
		Flags:          email.ImapFlags,
		DateReceived:   email.Date,
		Folder:         box,
	}

	return box.SaveBitmessage(bm)
}

// This error is used to cause mailbox.ForEachMessage to stop looping through
// every message once an ack is found, but is not a real error.
var errAckFound = errors.New("Ack Found")

// ReceiveAck takes an object payload and tests it against messages in the
// folder to see if it matches the ack of any sent message in the folder.
// The first such message found is returned.
func (box *Folder) ReceiveAck(ack []byte) *Bitmessage {
	var ackMatch *Bitmessage

	box.mailbox.ForEachMessage(0, 0, 2, func(id, suffix uint64, msg []byte) error {
		entry, err := DecodeBitmessage(msg)
		if err != nil {
			return err
		}

		if bytes.Equal(entry.Ack, ack) {
			ackMatch = entry

			// Stop ForEachMessage from searching the rest of the messages.
			return errors.New("Ack found")
		}
		return nil
	})

	if ackMatch == nil {
		return nil
	}

	ackMatch.AckReceived = true
	box.SaveBitmessage(ackMatch)

	return ackMatch
}

// NewMessage creates a new empty message associated with this folder.
// It is part of the email.ImapFolder interface.
func (box *Folder) NewMessage() mailstore.Message {
	return &email.ImapEmail{
		ImapFlags: types.FlagRecent,
		Folder:    box,
		Content:   &data.Content{},
	}
}

// Receive inserts a new message into the folder delivered by SMTP.
// It is a part of the mail.ImapFolder interface.
func (box *Folder) Receive(smtp *data.Message, flags types.Flags) (*email.ImapEmail, error) {
	bmsg, err := NewBitmessageFromSMTP(smtp.Content)
	if err != nil {
		return nil, err
	}

	entry, err := box.AddNew(bmsg, flags)
	if err != nil {
		return nil, err
	}

	return &email.ImapEmail{
		ImapSequenceNumber: entry.ImapData.SequenceNumber,
		ImapUID:            entry.ImapData.UID,
		ImapFlags:          flags,
		Date:               smtp.Created,
		Folder:             box,
		Content:            smtp.Content,
	}, nil
}

// Send sends the bitmessage with the given uid into the bitmessage network.
// TODO
func (box *Folder) Send(uid uint64) error {
	// Check whether the message has been sent.
	// Check whether the message has already had pow done.

	return nil
}

// NewFolder returns a new storebox.
func NewFolder(mailbox Mailbox) *Folder {
	var recent, unseen uint32
	list := list.New()

	// Run through every message to get the uids and count the
	// recent and unseen messages.
	if mailbox.ForEachMessage(0, 0, 2, func(id, suffix uint64, msg []byte) error {
		entry, err := DecodeBitmessage(msg)
		if err != nil {
			return err
		}
		recent += uint32(entry.ImapData.Flags & types.FlagRecent)
		unseen += uint32(entry.ImapData.Flags&types.FlagSeen) ^ 1

		list.PushBack(id)
		return nil
	}) != nil {
		return nil
	}

	uids := make([]uint64, list.Len())

	i := 0
	for e := list.Front(); e != nil; e = e.Next() {
		uids[i] = (e.Value).(uint64)
		i++
	}

	return &Folder{
		mailbox:   mailbox,
		numRecent: recent,
		numUnseen: unseen,
		uids:      uids,
	}
}
