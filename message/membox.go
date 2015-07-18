// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package message

/*import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jordwest/imap-server/mailstore"
	"github.com/jordwest/imap-server/types"
	"github.com/mailhog/data"
	"github.com/monetas/bmclient/message/email"
	"github.com/monetas/bmclient/message/format"
)

// membox is a BitmessageFolder that is stored in memory, mainly for testing purposes.
type membox struct {
	name    string
	nextUID uint32
	bm      []*BitmessageEntry
}

// Name gives name of the mailbox
// It is part of the BitmessageFolder interface.
func (box *membox) Name() string {
	return box.name
}

// NextUID returns the unique identifier that will LIKELY be assigned
// to the next mail that is added to this mailbox
// It is part of the BitmessageFolder interface.
func (box *membox) NextUID() uint32 {
	return box.nextUID
}

// LastUID returns the UID of the very last message in the mailbox
// If the mailbox is empty, this should return the next expected UID
// It is part of the BitmessageFolder interface.
func (box *membox) LastUID() uint32 {
	if len(box.bm) == 0 {
		return 0
	}
	return box.bm[len(box.bm)-1].UID
}

// Recent returns the number of recent messages in the mailbox
// It is part of the BitmessageFolder interface.
func (box *membox) Recent() uint32 {
	var count uint32
	for _, message := range box.bm {
		if message.Flags.HasFlags(types.FlagRecent) {
			count++
		}
	}
	return count
}

// Messages returns the umber of messages in the mailbox
// It is part of the BitmessageFolder interface.
func (box *membox) Messages() uint32 {
	return uint32(len(box.bm))
}

// Unseen gives the number of messages that do not have the Unseen flag set yet
// It is part of the BitmessageFolder interface.
func (box *membox) Unseen() uint32 {
	var count uint32
	for _, message := range box.bm {
		if !message.Flags.HasFlags(types.FlagSeen) {
			count++
		}
	}
	return count
}

// BitmessageBySequenceNumber returns a message by its sequence number.
// It is part of the BitmessageFolder interface.
func (box *membox) BitmessageBySequenceNumber(seqno uint32) *BitmessageEntry {
	if seqno > uint32(len(box.bm)) {
		return nil
	}
	return box.bm[seqno-1]
}

// Get a message by its uid number
// It is part of the BitmessageFolder interface.
func (box *membox) BitmessageByUID(uidno uint32) *BitmessageEntry {
	minIndex := 0
	maxIndex := len(box.bm) - 1

	// If the box is empty.
	if maxIndex < minIndex {
		return nil
	}

	// If there is one message in the box.
	if minIndex == maxIndex {
		if box.bm[minIndex].UID == uidno {
			return box.bm[minIndex]
		}
		return nil
	}

	minUID := box.bm[minIndex].UID
	maxUID := box.bm[maxIndex].UID

	for {
		if minIndex == maxIndex {
			return nil
		}

		ratio := (float64(uidno - minUID)) / (float64(maxUID - minUID))
		checkIndex := uint32(math.Floor(float64(minIndex) + ratio*float64(minIndex-maxIndex)))

		newUID := box.bm[checkIndex].UID

		if newUID == uidno {
			return box.bm[checkIndex]
		}

		if newUID > uidno {
			maxUID = newUID
		} else {
			// Add 1 because we use Floor function earlier.
			minUID = newUID + 1
		}
	}
}

// Get messages that belong to a set of ranges of UIDs
// It is part of the BitmessageFolder interface.
func (box *membox) BitmessageSetByUID(set types.SequenceSet) []*BitmessageEntry {
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
			msgs = append(msgs, box.bm[len(box.bm)-1])
			continue
		}

		start, err := msgRange.Min.Value()
		if err != nil {
			return msgs
		}

		// If no Max is specified, the sequence number must be either a fixed
		// sequence number or
		if msgRange.Max.Nil() {
			var uid uint32
			// Fetch specific message by sequence number
			uid, err = msgRange.Min.Value()
			msgs = append(msgs, box.BitmessageByUID(uid))
			if err != nil {
				return msgs
			}
			continue
		}

		var end uint32
		if msgRange.Max.Last() {
			end = box.LastUID()
		} else {
			end, err = msgRange.Max.Value()
		}

		// Note this is very inefficient when
		// the message array is large. A proper
		// storage system using eg SQL might
		// instead perform a query here using
		// the range values instead.
		for _, msg := range box.bm {
			uid := msg.UID
			if uid >= start && uid <= end {
				msgs = append(msgs, msg)
			}
		}
	}

	return msgs
}

// Get messages that belong to a set of ranges of sequence numbers.
// It is part of the BitmessageFolder interface.
func (box *membox) BitmessageSetBySequenceNumber(set types.SequenceSet) []*BitmessageEntry {
	var msgs []*BitmessageEntry

	// If the mailbox is empty, return empty array
	if box.Messages() == 0 {
		return msgs
	}

	// For each sequence range in the sequence set
	for _, msgRange := range set {
		// If Min is "*", meaning the last message in the mailbox, Max should
		// always be Nil
		if msgRange.Min.Last() {
			// Return the last message in the mailbox
			msgs = append(msgs, box.BitmessageBySequenceNumber(box.Messages()))
			continue
		}

		start, err := msgRange.Min.Value()
		if err != nil {
			return msgs
		}

		// If no Max is specified, the sequence number must be either a fixed
		// sequence number or
		if msgRange.Max.Nil() {
			var sequenceNo uint32
			// Fetch specific message by sequence number
			sequenceNo, err = msgRange.Min.Value()
			if err != nil {
				return msgs
			}
			msgs = append(msgs, box.BitmessageBySequenceNumber(sequenceNo))
			continue
		}

		var end uint32
		if msgRange.Max.Last() {
			end = uint32(len(box.bm))
		} else {
			end, err = msgRange.Max.Value()
		}

		// Note this is very inefficient when
		// the message array is large. A proper
		// storage system using eg SQL might
		// instead perform a query here using
		// the range values instead.
		for seqNo := start; seqNo <= end; seqNo++ {
			msgs = append(msgs, box.BitmessageBySequenceNumber(seqNo))
		}
	}
	return msgs
}

// AddNew creates a new message in a folder with the given flags.
// It is part of the BitmessageFolder interface.
func (box *membox) AddNew(msg *format.Encoding2, flags types.Flags) (*BitmessageEntry, error) {
	entry := &BitmessageEntry{
		SequenceNumber: uint32(len(box.bm)) + 1,
		UID:            box.nextUID,
		Flags:          flags,
		DateReceived:   time.Now(),
		Folder:         box,
		Message:        msg,
	}

	box.nextUID++

	box.bm = append(box.bm, entry)
	if len(box.bm) == cap(box.bm) {
		bm := make([]*BitmessageEntry, len(box.bm), 5*cap(box.bm))
		for i, b := range box.bm {
			bm[i] = b
		}
		box.bm = bm
	}
	fmt.Println("adding new message.... len(bm) =", len(box.bm))

	return entry, nil
}

// Save saves a bitmessage to the given uid in the folder.
// It is part of the BitmessageFolder interface.
func (box *membox) Save(msg *BitmessageEntry) error {
	// Check that the uid and sequence number are consistent with one another.
	previous := box.BitmessageBySequenceNumber(msg.SequenceNumber)
	if previous == nil {
		return errors.New("Invalid sequence number")
	}
	if previous.UID != msg.UID {
		return errors.New("Invalid uid")
	}

	box.bm[msg.SequenceNumber-1] = msg
	return nil
}

// Send sends the bitmessage corresponding to the given uid into the
// mysterious bitmessage network. It is part of the BitmessageFolder interface.
// TODO figure out how to send messages.
func (box *membox) Send(uid uint32) error {
	// Check whether the message exists.
	msg := box.BitmessageByUID(uid)
	if msg == nil {
		errors.New("Message not found")
	}

	// TODO make a way that the folders know whether they are for sending messages.
	return nil
}

// NewMembox returns a new membox.
func NewMembox(name string) BitmessageFolder {
	return &membox{
		name:    name,
		nextUID: 1,
		bm:      make([]*BitmessageEntry, 0, 20),
	}
}*/
