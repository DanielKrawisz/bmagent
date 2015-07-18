// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package message

import (
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/jordwest/imap-server/types"
	"github.com/monetas/bmclient/message/email"
	"github.com/monetas/bmclient/message/format"
	"github.com/monetas/bmclient/message/serialize"
)

// BitmessageEntry represents a bitmessage with related information
// attached as to its position and state in a folder.
type BitmessageEntry struct {
	UID            uint32
	SequenceNumber uint32
	Sent           bool
	AckExpected    bool
	AckReceived    bool
	DateReceived   time.Time
	Flags          types.Flags
	Folder         *Folder
	Message        format.Bitmessage
}

// ToEmail converts a BitmessageEntry into an ImapEmail.
func (be *BitmessageEntry) ToEmail() *email.ImapEmail {
	switch m := be.Message.(type) {
	case *format.Encoding2:
		return m.ToEmail(be.UID, be.SequenceNumber, be.DateReceived, be.Flags, be.Folder)
	default:
		return nil
	}
}

// Encode transforms a BitmessageEntry into a byte array suitable for inserting into the database.
func (be *BitmessageEntry) Encode() (uid uint32, sequenceNumber uint32, encoding uint64, msg []byte, err error) {
	date := be.DateReceived.Format(format.DateFormat)

	encode := &serialize.Entry{
		Sent:         &be.Sent,
		AckReceived:  &be.AckReceived,
		AckExpected:  &be.AckExpected,
		DateReceived: &date,
		Flags:        (*int32)(&be.Flags),
		Message:      be.Message.ToProtobuf(),
	}

	msg, err = proto.Marshal(encode)
	if err != nil {
		return 0, 0, 0, nil, err
	}
	return be.UID, be.SequenceNumber, 2, msg, nil
}

// DecodeBitmessageEntry reconstructs an entry from a byte slice.
func DecodeBitmessageEntry(uid, sequenceNumber uint32, msg []byte) (*BitmessageEntry, error) {
	entry := &serialize.Entry{}
	err := proto.Unmarshal(msg, entry)
	if err != nil {
		return nil, err
	}

	dateReceived, err := time.Parse(format.DateFormat, *entry.DateReceived)
	if err != nil {
		return nil, err
	}

	message, err := format.NewBitmessageFromProtobuf(entry.Message)
	if err != nil {
		return nil, err
	}

	be := &BitmessageEntry{
		AckExpected:  *entry.AckExpected,
		AckReceived:  *entry.AckReceived,
		Sent:         *entry.Sent,
		Flags:        types.Flags(*entry.Flags),
		DateReceived: dateReceived,
		Message:      message,
	}
	return be, nil
}
