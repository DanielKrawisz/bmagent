package message

import (
	"math"
	"time"

	"github.com/jordwest/imap-server/types"
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

// Storebox implements the BitmessageFolder interface and it uses store.Mailbox
// to store messages.
type Storebox struct {
	mailbox   *store.Mailbox
	uids      []uint64
	numRecent uint32
	numUnseen uint32
}

// Name returns the name of the mailbox.
// This is part of the BitmessageFolder interface.
func (box *Storebox) Name() string {
	return box.mailbox.GetName()
}

// NextUID returns the unique identifier that will LIKELY be assigned
// to the next mail that is added to this mailbox.
// This is part of the BitmessageFolder interface.
func (box *Storebox) NextUID() uint32 {
	last, err := box.mailbox.GetLastID()
	if err != nil {
		return 1
	}
	return uint32(last) + 1
}

// LastUID assigns the UID of the very last message in the mailbox
// If the mailbox is empty, this should return the next expected UID.
// This is part of the BitmessageFolder interface.
func (box *Storebox) LastUID() uint32 {
	last, err := box.mailbox.GetLastIDBySuffix(encoding2)
	if err != nil {
		return 0
	}
	return uint32(last)
}

// Recent returns the number of recent messages in the mailbox
// This is part of the BitmessageFolder interface.
func (box *Storebox) Recent() uint32 {
	return box.numRecent
}

// Messages returns the number of messages in the mailbox
// This is part of the BitmessageFolder interface.
func (box *Storebox) Messages() uint32 {
	return uint32(len(box.uids))
}

// Unseen returns the number of messages that do not have the Unseen flag set yet
// This is part of the BitmessageFolder interface.
func (box *Storebox) Unseen() uint32 {
	return box.numUnseen
}

// BitmessageBySequenceNumber gets a message by its sequence number
// This is part of the BitmessageFolder interface.
func (box *Storebox) BitmessageBySequenceNumber(seqno uint32) *BitmessageEntry {
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

// BitmessageByUID returns a message by its uid.
// This is part of the BitmessageFolder interface.
func (box *Storebox) BitmessageByUID(uidno uint32) *BitmessageEntry {
	suffix, msg, err := box.mailbox.GetMessage(uint64(uidno))
	if err != nil || suffix != 2 {
		return nil
	}
	seqno := GetSequenceNumber(box.uids, uint64(uidno))
	b, _ := DecodeBitmessageEntry(uidno, seqno, msg)
	return b
}

// LastBitmessage returns the last Bitmessage in the mailbox.
func (box *Storebox) LastBitmessage() *BitmessageEntry {
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
func (box *Storebox) getRange(startUID, startSequence, endUID, endSequence uint32) []*BitmessageEntry {
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
func (box *Storebox) getSince(startUID, startSequence uint32) []*BitmessageEntry {
	return box.getRange(startUID, startSequence, 0, box.Messages())
}

// BitmessagesByUIDRange returns the last Bitmessage in the mailbox.
func (box *Storebox) BitmessagesByUIDRange(start uint32, end uint32) []*BitmessageEntry {
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
func (box *Storebox) BitmessagesSinceUID(start uint32) []*BitmessageEntry {
	startSequence := GetSequenceNumber(box.uids, uint64(start))
	if startSequence == 0 {
		return nil
	}
	return box.getSince(start, startSequence)
}

// BitmessagesBySequenceRange returns the last Bitmessage in the mailbox.
func (box *Storebox) BitmessagesBySequenceRange(start uint32, end uint32) []*BitmessageEntry {
	if start < 1 || start > box.Messages() || end < 1 || end > box.Messages() || end < start {
		return nil
	}
	startUID := uint32(box.uids[start])
	endUID := uint32(box.uids[end])
	return box.getRange(startUID, start, endUID, end)
}

// BitmessagesSinceSequenceNumber returns the last Bitmessage in the mailbox.
func (box *Storebox) BitmessagesSinceSequenceNumber(start uint32) []*BitmessageEntry {
	if start < 1 || start > box.Messages() {
		return nil
	}
	startUID := uint32(box.uids[start])
	return box.getSince(startUID, start)
}

// BitmessageSetByUID gets messages belonging to a set of ranges of UIDs
// This is part of the BitmessageFolder interface.
func (box *Storebox) BitmessageSetByUID(set types.SequenceSet) []*BitmessageEntry {
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
// This is part of the BitmessageFolder interface.
func (box *Storebox) BitmessageSetBySequenceNumber(set types.SequenceSet) []*BitmessageEntry {
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
// This is part of the BitmessageFolder interface.
func (box *Storebox) AddNew(bmsg Bitmessage, flags types.Flags) (*BitmessageEntry, error) {
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

	uid, err := box.mailbox.InsertMessage(msg, encoding)
	if err != nil {
		return nil, err
	}

	entry.UID = uint32(uid)
	box.uids = append(box.uids, uid)

	box.numRecent++
	box.numUnseen++

	return entry, nil
}

// Save saves the given bitmessage entry in the folder.
// This is part of the BitmessageFolder interface.
// TODO Need to make a way for mailbox to save over messages.
func (box *Storebox) Save(msg *BitmessageEntry) error {
	return nil
}

// Send sends the bitmessage with the given uid into the bitmessage network.
// This is part of the BitmessageFolder interface.
// TODO
func (box *Storebox) Send(uid uint32) error {
	return nil
}

// NewStorebox returns a new storebox.
func NewStorebox(mailbox *store.Mailbox) *Storebox {
	var recent, unseen, sequenceNumber uint32
	var uids []uint64

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

		// TODO this is pretty inefficient. We should be able to learn how
		// many messages there are of a given type.
		uids = append(uids, id)

		sequenceNumber++
		return nil
	}) != nil {
		return nil
	}

	return &Storebox{
		mailbox:   mailbox,
		numRecent: recent,
		numUnseen: unseen,
		uids:      uids,
	}
}
