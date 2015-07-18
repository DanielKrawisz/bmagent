// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package format

import (
	"time"

	"github.com/monetas/bmclient/message/serialize"
)

// Encoding1 implements the Bitmessage interface and represents a
// MsgMsg or MsgBroadcast with encoding type 1.
type Encoding1 struct {
	from       *string
	to         *string
	body       string
	expiration *time.Time
}

// To returns the to bitmessage address.
// Part of the Bitmessage interface.
func (l *Encoding1) To() *string {
	return l.to
}

// From returns the from bitmessage address.
// Part of the Bitmessage interface.
func (l *Encoding1) From() *string {
	return l.from
}

// Expiration returns the expiration time of the message. May be nil.
func (l *Encoding1) Expiration() *time.Time {
	return l.expiration
}

// Encoding returns the encoding format of the bitmessage.
func (l *Encoding1) Encoding() uint64 {
	return 1
}

// Message returns the raw form of the object payload.
func (l *Encoding1) Message() []byte {
	return []byte(l.body)
}

// ToProtobuf encodes the message in a protobuf format.
func (l *Encoding1) ToProtobuf() *serialize.Encoding {
	encoding := uint64(1)
	var expr string
	if l.expiration != nil {
		expr = (*l.expiration).Format(DateFormat)
	}
	return &serialize.Encoding{
		Encoding:   &encoding,
		From:       l.from,
		To:         l.to,
		Body:       &l.body,
		Expiration: &expr,
	}
}
