// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

import (
	"bytes"
	"container/list"
	"errors"
	"math"
	"time"

	"github.com/jordwest/imap-server/mailstore"
	"github.com/jordwest/imap-server/types"
	"github.com/mailhog/data"
	"github.com/monetas/bmclient/store"
)

// GetSequenceNumber gets the highest sequence number lower than or equal to
// the given uid.
func GetSequenceNumber(uids []uint64, uid uint64) (uint32, uint64) {
	// If the slice is empty.
	if len(uids) == 0 {
		return 0, 0
	}

	// If there is only one message in the box.
	if len(uids) == 1 {
		if uids[0] == uid {
			return 1, uids[0]
		}
		if uids[0] < uid {
			return 1, uids[0]
		}
		return 0, 0
	}

	var minIndex uint32
	maxIndex := uint32(len(uids) - 1)

	minUID := uids[minIndex]
	maxUID := uids[maxIndex]

	for {
		ratio := (float64(uid - minUID)) / (float64(maxUID - minUID))
		checkIndex := uint32(math.Floor(float64(minIndex) + ratio*float64(maxIndex-minIndex)))

		newUID := uids[checkIndex]

		if newUID == uid {
			return checkIndex + 1, uid
		}

		if newUID > uid {
			maxUID = newUID
		} else {
			// Add 1 because we use Floor function earlier.
			minUID = newUID + 1
		}

		if minIndex == maxIndex {
			if newUID < uid {
				return checkIndex, newUID
			}
			return checkIndex - 1, newUID
		}
	}
}

// Mailbox implements a mailbox that is compatible with IMAP. It implements the
// email.IMAPMailbox interface.
type Mailbox struct {
	mbox      *store.Mailbox
	uids      []uint64
	numRecent uint32
	numUnseen uint32
}

func (box *Mailbox) decodeBitmessageForImap(uid uint64, seqno uint32, msg []byte) *Bitmessage {
	b, _ := DecodeBitmessage(msg)
	if b == nil {
		return nil
	}
	b.ImapData.UID = uid
	b.ImapData.SequenceNumber = seqno
	b.ImapData.Mailbox = box
	return b
}

// Name returns the name of the mailbox.
// This is part of the email.ImapFolder interface.
func (box *Mailbox) Name() string {
	return box.mbox.Name()
}

// NextUID returns the unique identifier that will LIKELY be assigned
// to the next mail that is added to this mailbox.
// This is part of the email.ImapFolder interface.
func (box *Mailbox) NextUID() uint32 {
	n, _ := box.mbox.NextID()
	return uint32(n)
}

// LastUID assigns the UID of the very last message in the mailbox
// If the mailbox is empty, this should return the next expected UID.
// This is part of the email.ImapFolder interface.
func (box *Mailbox) LastUID() uint32 {
	last, err := box.mbox.LastIDBySuffix(2)
	if err != nil {
		return box.NextUID()
	}
	return uint32(last)
}

// Recent returns the number of recent messages in the mailbox
// This is part of the email.ImapFolder interface.
func (box *Mailbox) Recent() uint32 {
	return box.numRecent
}

// Messages returns the number of messages in the mailbox
// This is part of the email.ImapFolder interface.
func (box *Mailbox) Messages() uint32 {
	return uint32(len(box.uids))
}

// Unseen returns the number of messages that do not have the Unseen flag set yet
// This is part of the email.ImapFolder interface.
func (box *Mailbox) Unseen() uint32 {
	return box.numUnseen
}

// BitmessageBySequenceNumber gets a message by its sequence number
func (box *Mailbox) BitmessageBySequenceNumber(seqno uint32) *Bitmessage {
	if seqno < 1 || seqno > box.Messages() {
		return nil
	}
	uid := box.uids[seqno-1]
	suffix, msg, err := box.mbox.GetMessage(uid)
	if err != nil || suffix != 2 {
		return nil
	}

	return box.decodeBitmessageForImap(uid, seqno, msg)
}

// MessageBySequenceNumber gets a message by its sequence number
// It is a part of the mail.SMTPFolder interface.
func (box *Mailbox) MessageBySequenceNumber(seqno uint32) mailstore.Message {
	bm := box.BitmessageBySequenceNumber(seqno)
	if bm == nil {
		return nil
	}
	email, err := bm.ToEmail()
	if err != nil {
		imapLog.Error("MessageBySequenceNumber (%d) gave error %v", seqno, err)
		return nil
	}

	return email
}

// BitmessageByUID returns a message by its uid.
func (box *Mailbox) BitmessageByUID(uidno uint64) *Bitmessage {
	suffix, msg, err := box.mbox.GetMessage(uint64(uidno))
	if err != nil || suffix != 2 {
		return nil
	}
	seqno, _ := GetSequenceNumber(box.uids, uint64(uidno))

	return box.decodeBitmessageForImap(uidno, seqno, msg)
}

// MessageByUID gets a message by its uid number
// It is a part of the mail.SMTPFolder interface.
func (box *Mailbox) MessageByUID(uidno uint32) mailstore.Message {
	letter := box.BitmessageByUID(uint64(uidno))
	if letter == nil {
		return nil
	}
	email, _ := letter.ToEmail()
	return email
}

// LastBitmessage returns the last Bitmessage in the mailbox.
func (box *Mailbox) LastBitmessage() *Bitmessage {
	if len(box.uids) == 0 {
		return nil
	}

	uid := box.uids[len(box.uids)-1]
	_, msg, _ := box.mbox.GetMessage(uid)
	return box.decodeBitmessageForImap(uid, uint32(len(box.uids)), msg)
}

// getRange returns a sequence of bitmessages from the mailbox in a range from
// startUID to endUID. It does not check whether the given sequence numbers make sense.
func (box *Mailbox) getRange(startUID, endUID uint64, startSequence, endSequence uint32) []*Bitmessage {
	bitmessages := make([]*Bitmessage, endSequence-startSequence+1)

	var i uint32

	gen := func(id, suffix uint64, msg []byte) error {
		if suffix != 2 {
			return nil
		}
		bm := box.decodeBitmessageForImap(id, startSequence+i, msg)
		if bm == nil {
			return errors.New("Message should not be nil")
		}
		bitmessages[i] = bm
		i++
		return nil
	}

	err := box.mbox.ForEachMessage(startUID, endUID, 2, gen)
	if err != nil {
		return nil
	}
	return bitmessages[:i]
}

// getSince returns a sequence of bitmessages from the mailbox which includes
// all greater than or equal to a given uid number. It does not check whether
// the given sequence number makes sense.
func (box *Mailbox) getSince(startUID uint64, startSequence uint32) []*Bitmessage {
	return box.getRange(startUID, 0, startSequence, box.Messages())
}

// BitmessagesByUIDRange returns the last Bitmessage in the mailbox.
func (box *Mailbox) BitmessagesByUIDRange(start, end uint64) []*Bitmessage {
	startSequence, startUID := GetSequenceNumber(box.uids, start)
	if startUID != start {
		startSequence++
	}
	endSequence, _ := GetSequenceNumber(box.uids, end)

	if startSequence > endSequence {
		return []*Bitmessage{}
	}
	return box.getRange(start, end, startSequence, endSequence)
}

// BitmessagesSinceUID returns the last Bitmessage in the mailbox.
func (box *Mailbox) BitmessagesSinceUID(start uint64) []*Bitmessage {
	startSequence, startUID := GetSequenceNumber(box.uids, start)
	if startUID != start {
		startSequence++
	}
	return box.getSince(start, startSequence)
}

// BitmessagesBySequenceRange returns a set of Bitmessages in a range between two sequence numbers inclusive.
func (box *Mailbox) BitmessagesBySequenceRange(start, end uint32) []*Bitmessage {
	if start < 1 || start > box.Messages() || end < 1 || end > box.Messages() || end < start {
		return nil
	}
	startUID := box.uids[start]
	endUID := box.uids[end]
	return box.getRange(startUID, endUID, start, end)
}

// BitmessagesSinceSequenceNumber returns the set of Bitmessages since and including a given uid value.
func (box *Mailbox) BitmessagesSinceSequenceNumber(start uint32) []*Bitmessage {
	if start < 1 || start > box.Messages() {
		return nil
	}
	startUID := box.uids[start]
	return box.getSince(startUID, start)
}

// BitmessageSetByUID gets messages belonging to a set of ranges of UIDs
func (box *Mailbox) BitmessageSetByUID(set types.SequenceSet) []*Bitmessage {
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
	return msgs
}

// BitmessageSetBySequenceNumber gets messages belonging to a set of ranges of sequence numbers
func (box *Mailbox) BitmessageSetBySequenceNumber(set types.SequenceSet) []*Bitmessage {
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

func (box *Mailbox) updateUIDs(uid uint64) {
	box.uids = append(box.uids, uid)

	box.numRecent++
	box.numUnseen++
}

// AddNew adds a new Bitmessage to the Mailbox.
func (box *Mailbox) AddNew(bmsg *Bitmessage, flags types.Flags) error {
	encoding := bmsg.Payload.Encoding()
	if encoding != 2 {
		return errors.New("Unsupported encoding")
	}

	imapData := &IMAPData{
		SequenceNumber: uint32(len(box.uids)),
		Flags:          flags,
		DateReceived:   time.Now(),
		Mailbox:        box,
	}

	bmsg.ImapData = imapData

	msg, err := bmsg.Serialize()
	if err != nil {
		return err
	}

	uid, err := box.mbox.InsertMessage(msg, 0, bmsg.Payload.Encoding())
	if err != nil {
		return err
	}

	imapData.UID = uid
	box.updateUIDs(uid)

	return nil
}

// MessageSetByUID returns the slice of messages belonging to a set of ranges of UIDs
// It is a part of the mail.SMTPFolder interface.
func (box *Mailbox) MessageSetByUID(set types.SequenceSet) []mailstore.Message {
	msgs := box.BitmessageSetByUID(set)
	email := make([]mailstore.Message, len(msgs))
	for _, msg := range msgs {
		email[msg.ImapData.SequenceNumber-1], _ = msg.ToEmail()
	}
	return email
}

// MessageSetBySequenceNumber returns the slice of messages belonging to a set
// of ranges of sequence numbers
// It is a part of the mail.SMTPFolder interface.
func (box *Mailbox) MessageSetBySequenceNumber(set types.SequenceSet) []mailstore.Message {
	msgs := box.BitmessageSetBySequenceNumber(set)
	email := make([]mailstore.Message, len(msgs))
	for i, msg := range msgs {
		email[i], _ = msg.ToEmail()
	}
	return email
}

// Save saves the given bitmessage entry in the folder.
func (box *Mailbox) SaveBitmessage(msg *Bitmessage) error {
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
			return errors.New("Cannot change date received")
		}

		// Delete the old message from the database.
		err := box.mbox.DeleteMessage(uint64(msg.ImapData.UID))
		if err != nil {
			return err
		}
	}

	// Generate the new version of the message.
	encode, err := msg.Serialize()
	if err != nil {
		return err
	}

	// Insert the new version of the message.
	newUID, err := box.mbox.InsertMessage(encode, msg.ImapData.UID, msg.Payload.Encoding())
	if err != nil {
		return err
	}

	if msg.ImapData.UID == 0 {
		msg.ImapData.UID = newUID
		box.updateUIDs(newUID)
	}
	return nil
}

// Save saves an IMAP email in the Mailbox. It is part of the IMAPMailbox
// interface.
func (box *Mailbox) Save(email *IMAPEmail) error {
	bm, err := NewBitmessageFromSMTP(email.Content)
	if err != nil {
		return err
	}

	bm.ImapData = &IMAPData{
		UID:            email.ImapUID,
		SequenceNumber: email.ImapSequenceNumber,
		Flags:          email.ImapFlags,
		DateReceived:   email.Date,
		Mailbox:        box,
	}

	return box.SaveBitmessage(bm)
}

// This error is used to cause mailbox.ForEachMessage to stop looping through
// every message once an ack is found, but is not a real error.
var errAckFound = errors.New("Ack Found")

// ReceiveAck takes an object payload and tests it against messages in the
// folder to see if it matches the ack of any sent message in the folder.
// The first such message found is returned.
func (box *Mailbox) ReceiveAck(ack []byte) *Bitmessage {
	var ackMatch *Bitmessage

	box.mbox.ForEachMessage(0, 0, 2, func(id, suffix uint64, msg []byte) error {
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
// It is part of the IMAPMailbox interface.
func (box *Mailbox) NewMessage() mailstore.Message {
	return &IMAPEmail{
		ImapFlags: types.FlagRecent,
		Mailbox:   box,
		Content:   &data.Content{},
	}
}

// Receive inserts a new message into the folder delivered by SMTP.
// It is a part of the mail.ImapFolder interface.
/*func (box *Mailbox) Receive(smtp *data.Message, flags types.Flags) (*IMAPEmail, error) {
	bmsg, err := NewBitmessageFromSMTP(smtp.Content)
	if err != nil {
		return nil, err
	}

	entry, err := box.AddNew(bmsg, flags)
	if err != nil {
		return nil, err
	}

	return &IMAPEmail{
		ImapSequenceNumber: entry.ImapData.SequenceNumber,
		ImapUID:            entry.ImapData.UID,
		ImapFlags:          flags,
		Date:               smtp.Created,
		Mailbox:            box,
		Content:            smtp.Content,
	}, nil
}*/

// NewMailbox returns a new mailbox.
func NewMailbox(mbox *store.Mailbox) (*Mailbox, error) {
	var recent, unseen uint32
	list := list.New()

	// Run through every message to get the uids and count the
	// recent and unseen messages.
	err := mbox.ForEachMessage(0, 0, 2, func(id, suffix uint64, msg []byte) error {
		entry, err := DecodeBitmessage(msg)
		if err != nil {
			imapLog.Errorf("Failed to decode message #%d: %v", id, err)
			return nil
		}
		recent += uint32(entry.ImapData.Flags & types.FlagRecent)
		unseen += uint32(entry.ImapData.Flags&types.FlagSeen) ^ 1

		list.PushBack(id)
		return nil
	})
	if err != nil {
		return nil, err
	}

	uids := make([]uint64, list.Len())

	i := 0
	for e := list.Front(); e != nil; e = e.Next() {
		uids[i] = (e.Value).(uint64)
		i++
	}

	return &Mailbox{
		mbox:      mbox,
		numRecent: recent,
		numUnseen: unseen,
		uids:      uids,
	}, nil
}
