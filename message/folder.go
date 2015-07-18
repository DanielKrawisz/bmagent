// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package message

import (
	"container/list"
	"errors"
	"math"
	"time"

	"github.com/jordwest/imap-server/mailstore"
	"github.com/jordwest/imap-server/types"
	"github.com/mailhog/data"
	"github.com/monetas/bmclient/message/email"
	"github.com/monetas/bmclient/message/format"
	"github.com/monetas/bmclient/store"
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

// Folder implements a bitmessage folder that is compatible with imap.
// It implements the email.ImapFolder interface.
type Folder struct {
	mailbox   *store.Mailbox
	uids      []uint64
	numRecent uint32
	numUnseen uint32
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
	last, err := box.mailbox.GetLastID()
	if err != nil {
		return 1
	}
	return uint32(last) + 1
}

// LastUID assigns the UID of the very last message in the mailbox
// If the mailbox is empty, this should return the next expected UID.
// This is part of the email.ImapFolder interface.
func (box *Folder) LastUID() uint32 {
	last, err := box.mailbox.GetLastIDBySuffix(encoding2)
	if err != nil {
		return 0
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
func (box *Folder) BitmessageBySequenceNumber(seqno uint32) *BitmessageEntry {
	if seqno < 1 || seqno > box.Messages() {
		return nil
	}
	uid := box.uids[seqno-1]
	suffix, msg, err := box.mailbox.GetMessage(uid)
	if err != nil || suffix != 2 {
		return nil
	}

	b, _ := DecodeBitmessageEntry(uint32(uid), seqno, msg)
	return b
}

// MessageBySequenceNumber gets a message by its sequence number
// It is a part of the mail.SMTPFolder interface.
func (box *Folder) MessageBySequenceNumber(seqno uint32) mailstore.Message {
	letter := box.BitmessageBySequenceNumber(seqno)
	if letter == nil {
		return nil
	}
	return letter.ToEmail()
}

// BitmessageByUID returns a message by its uid.
func (box *Folder) BitmessageByUID(uidno uint32) *BitmessageEntry {
	suffix, msg, err := box.mailbox.GetMessage(uint64(uidno))
	if err != nil || suffix != 2 {
		return nil
	}
	seqno := GetSequenceNumber(box.uids, uint64(uidno))
	b, _ := DecodeBitmessageEntry(uidno, seqno, msg)
	return b
}

// MessageByUID gets a message by its uid number
// It is a part of the mail.SMTPFolder interface.
func (box *Folder) MessageByUID(uidno uint32) mailstore.Message {
	letter := box.BitmessageByUID(uidno)
	if letter == nil {
		return nil
	}
	return letter.ToEmail()
}

// LastBitmessage returns the last Bitmessage in the mailbox.
func (box *Folder) LastBitmessage() *BitmessageEntry {
	if len(box.uids) == 0 {
		return nil
	}

	uid := box.uids[len(box.uids)-1]
	_, msg, _ := box.mailbox.GetMessage(uid)
	b, _ := DecodeBitmessageEntry(uint32(uid), uint32(len(box.uids)), msg)
	return b
}

// getRange returns a sequence of bitmessages from the mailbox in a range from
// startUID to endUID. It does not check whether the given sequence numbers make sense.
func (box *Folder) getRange(startUID, startSequence, endUID, endSequence uint32) []*BitmessageEntry {
	bitmessages := make([]*BitmessageEntry, endSequence-startSequence+1)

	var i uint32

	gen := func(id, suffix uint64, msg []byte) error {
		if suffix != 2 {
			return nil
		}

		bitmessages[i], _ = DecodeBitmessageEntry(uint32(id), startSequence+i, msg)
		i++
		return nil
	}

	if box.mailbox.ForEachMessage(uint64(startUID), uint64(endUID), 2, gen) != nil {
		return nil
	}
	return bitmessages
}

// getSince returns a sequence of bitmessages from the mailbox which includes
// all greater than or equal to a given uid number. It does not check whether
// the given sequence number makes sense.
func (box *Folder) getSince(startUID, startSequence uint32) []*BitmessageEntry {
	return box.getRange(startUID, startSequence, 0, box.Messages())
}

// BitmessagesByUIDRange returns the last Bitmessage in the mailbox.
func (box *Folder) BitmessagesByUIDRange(start uint32, end uint32) []*BitmessageEntry {
	startSequence := GetSequenceNumber(box.uids, uint64(start))
	if startSequence == 0 {
		return nil
	}
	endSequence := GetSequenceNumber(box.uids, uint64(end))
	if endSequence == 0 || endSequence < startSequence {
		return nil
	}
	return box.getRange(start, startSequence, end, endSequence)
}

// BitmessagesSinceUID returns the last Bitmessage in the mailbox.
func (box *Folder) BitmessagesSinceUID(start uint32) []*BitmessageEntry {
	startSequence := GetSequenceNumber(box.uids, uint64(start))
	if startSequence == 0 {
		return nil
	}
	return box.getSince(start, startSequence)
}

// BitmessagesBySequenceRange returns a set of Bitmessages in a range between two sequence numbers inclusive.
func (box *Folder) BitmessagesBySequenceRange(start uint32, end uint32) []*BitmessageEntry {
	if start < 1 || start > box.Messages() || end < 1 || end > box.Messages() || end < start {
		return nil
	}
	startUID := uint32(box.uids[start])
	endUID := uint32(box.uids[end])
	return box.getRange(startUID, start, endUID, end)
}

// BitmessagesSinceSequenceNumber returns the set of Bitmessages since and including a given uid value.
func (box *Folder) BitmessagesSinceSequenceNumber(start uint32) []*BitmessageEntry {
	if start < 1 || start > box.Messages() {
		return nil
	}
	startUID := uint32(box.uids[start])
	return box.getSince(startUID, start)
}

// BitmessageSetByUID gets messages belonging to a set of ranges of UIDs
func (box *Folder) BitmessageSetByUID(set types.SequenceSet) []*BitmessageEntry {
	var msgs []*BitmessageEntry

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
			msgs = append(msgs, box.BitmessageByUID(start))
			if err != nil {
				return msgs
			}
			continue
		}

		var end uint32
		if msgRange.Max.Last() {
			msgs = append(msgs, box.BitmessagesSinceUID(start)...)
		} else {
			end, err = msgRange.Max.Value()
			if err != nil {
				return msgs
			}
			msgs = append(msgs, box.BitmessagesByUIDRange(start, end)...)
		}
	}

	return msgs
}

// BitmessageSetBySequenceNumber gets messages belonging to a set of ranges of sequence numbers
func (box *Folder) BitmessageSetBySequenceNumber(set types.SequenceSet) []*BitmessageEntry {
	var msgs []*BitmessageEntry

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

// AddNew adds a new bitmessage to the storebox.
func (box *Folder) AddNew(bmsg *format.Encoding2, flags types.Flags) (*BitmessageEntry, error) {
	entry := &BitmessageEntry{
		SequenceNumber: uint32(len(box.uids)),
		Flags:          flags,
		DateReceived:   time.Now(),
		Folder:         box,
		Message:        bmsg,
	}

	_, _, encoding, msg, err := entry.Encode()
	if err != nil {
		return nil, err
	}

	uid, err := box.mailbox.InsertMessage(msg, encoding, 0)
	if err != nil {
		return nil, err
	}

	entry.UID = uint32(uid)
	box.uids = append(box.uids, uid)

	box.numRecent++
	box.numUnseen++

	return entry, nil
}

// MessageSetByUID returns the slice of messages belonging to a set of ranges of UIDs
// It is a part of the mail.SMTPFolder interface.
func (box *Folder) MessageSetByUID(set types.SequenceSet) []mailstore.Message {
	msgs := box.BitmessageSetByUID(set)
	email := make([]mailstore.Message, len(msgs))
	for _, msg := range msgs {
		email[msg.SequenceNumber-1] = msg.ToEmail()
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
		email[i] = msg.ToEmail()
	}
	return email
}

// Save saves the given bitmessage entry in the folder.
func (box *Folder) SaveBitmessage(msg *BitmessageEntry) error {
	// Check that the uid and sequence number are consistent with one another.
	previous := box.BitmessageBySequenceNumber(msg.SequenceNumber)
	if previous == nil {
		return errors.New("Invalid sequence number")
	}
	if previous.UID != msg.UID {
		return errors.New("Invalid uid")
	}

	// Delete the old message from the database.
	err := box.mailbox.DeleteMessage(uint64(msg.UID))
	if err != nil {
		return err
	}

	// Generate the new version of the message.
	_, _, _, encode, err := msg.Encode()
	if err != nil {
		return err
	}

	// insert the new version of the message.
	_, err = box.mailbox.InsertMessage(encode, uint64(msg.UID), encoding2)
	if err != nil {
		return err
	}
	return nil
}

// Save saves an imap email to its folder.
// It is a part of the mail.SMTPFolder interface.
func (box *Folder) Save(email *email.ImapEmail) error {
	entry := box.BitmessageByUID(email.ImapUID)

	bm, err := format.NewBitmessageFromSMTP(email.Content)
	if err != nil {
		return err
	}

	entry.Flags = email.ImapFlags
	entry.Message = bm

	return box.SaveBitmessage(entry)
}

// NewMessage inserts a new message into the folder.
// It is a part of the mail.SMTPFolder interface.
func (box *Folder) NewMessage(smtp *data.Message, flags types.Flags) (*email.ImapEmail, error) {
	bmsg, err := format.NewBitmessageFromSMTP(smtp.Content)
	if err != nil {
		return nil, err
	}

	entry, err := box.AddNew(bmsg, flags)
	if err != nil {
		return nil, err
	}

	return &email.ImapEmail{
		ImapSequenceNumber: entry.SequenceNumber,
		ImapUID:            entry.UID,
		ImapFlags:          flags,
		ImapDate:           smtp.Created,
		ImapFolder:         box,
		Content:            smtp.Content,
	}, nil
}

// Send sends the bitmessage with the given uid into the bitmessage network.
// TODO
func (box *Folder) Send(uid uint32) error {
	return nil
}

// NewStorebox returns a new storebox.
func NewFolder(mailbox *store.Mailbox) *Folder {
	var recent, unseen, sequenceNumber uint32
	list := list.New()

	// Run through every message to get the uids and count the
	// recent and unseen messages.
	sequenceNumber = 1
	if mailbox.ForEachMessage(0, 0, 2, func(id, suffix uint64, msg []byte) error {
		entry, err := DecodeBitmessageEntry(uint32(id), sequenceNumber, msg)
		if err != nil {
			return err
		}
		recent += uint32(entry.Flags & types.FlagRecent)
		unseen += uint32(entry.Flags&types.FlagSeen) ^ 1

		list.PushBack(id)

		sequenceNumber++
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
