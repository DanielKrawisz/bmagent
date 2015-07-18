// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package format

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/monetas/bmclient/keymgr"
	"github.com/monetas/bmclient/message/serialize"
	"github.com/monetas/bmutil/identity"
	"github.com/monetas/bmutil/wire"
)

// The format for encoding dates.
const (
	DateFormat = "Mon Jan 2 15:04:05 -0700 MST 2006"

	bmAddrPattern = "BM-[123456789abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ]+"

	broadcastAddress = "broadcast@bm.addr"
)

var (
	bmAddrRegex, _ = regexp.Compile(bmAddrPattern)
	emailRegex, _  = regexp.Compile(fmt.Sprintf("%s@bm\\.addr", bmAddrRegex))
)

// Bitmessage represents a message compatible with a Bitmessage format.
type Bitmessage interface {
	// To returns the bm address the message is to be sent to.
	// It will be nil if the message is a broadcast.
	To() *string
	From() *string
	Encoding() uint64
	Expiration() *time.Time
	// Message returns the encoded form of the object payload.
	Message() []byte
	ToProtobuf() *serialize.Encoding
}

// NewBitmessageFromProtobuf takes the protobuf encoding of a bitmessage
// and converts it back to a bitmessage.
func NewBitmessageFromProtobuf(msg *serialize.Encoding) (Bitmessage, error) {
	switch *msg.Encoding {
	case 1:
		r := &Encoding1{}

		if msg.Body == nil {
			return nil, errors.New("Body required in encoding format 2")
		}
		r.body = *msg.Body

		if msg.Expiration != nil {
			expr, _ := time.Parse(DateFormat, *msg.Expiration)
			r.expiration = &expr
		}

		r.from = msg.From
		r.to = msg.To

		return r, nil
	case 2:
		r := &Encoding2{}

		if msg.Subject == nil {
			return nil, errors.New("Subject required in encoding format 2")
		}
		r.subject = *msg.Subject

		if msg.Body == nil {
			return nil, errors.New("Body required in encoding format 2")
		}
		r.body = *msg.Body

		if msg.Expiration != nil {
			expr, _ := time.Parse(DateFormat, *msg.Expiration)
			r.expiration = &expr
		}

		r.from = msg.From
		r.to = msg.To

		return r, nil
	default:
		return nil, errors.New("Unsupported encoding")
	}
}

// TODO
func generateMsgBroadcast(content []byte, from *identity.Private) (wire.Message, error) {
	return nil, nil
}

// TODO
func generateMsgMsg(content []byte, from *identity.Private, to *identity.Public) (wire.Message, error) {
	return nil, nil
}

// ToMessage converts the message to wire format.
func ToMessage(msg Bitmessage, book keymgr.AddressBook) (wire.Message, error) {
	msgContent := msg.Message()
	from, err := book.LookupPrivateIdentity(msg.From())
	if err != nil {
		return nil, err
	}

	toAddr := msg.To()
	if toAddr == nil {
		return generateMsgBroadcast(msgContent, from)
	}

	to, err := book.LookupPublicIdentity(toAddr)
	if err != nil {
		return nil, err
	}

	return generateMsgMsg(msgContent, from, to)
}
