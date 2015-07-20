// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package format

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/monetas/bmclient/message/serialize"
)

var encoding2Regex = regexp.MustCompile("$Subject:([.\\n\\r]+)\nBody:([.\\n\\r]+)^")

// Encoding represents a msg or broadcast object payload.
type Encoding interface {
	Encoding() uint64
	Message() []byte
	ReadMessage([]byte) error
	ToProtobuf() *serialize.Encoding
}

// Encoding1 implements the Bitmessage interface and represents a
// MsgMsg or MsgBroadcast with encoding type 1.
type Encoding1 struct {
	Body string
}

// Encoding returns the encoding format of the bitmessage.
func (l *Encoding1) Encoding() uint64 {
	return 1
}

// Message returns the raw form of the object payload.
func (l *Encoding1) Message() []byte {
	return []byte(l.Body)
}

// ReadMessage reads the object payload and incorporates it.
func (l *Encoding1) ReadMessage(msg []byte) error {
	l.Body = string(msg)
	return nil
}

// ToProtobuf encodes the message in a protobuf format.
func (l *Encoding1) ToProtobuf() *serialize.Encoding {
	format := uint64(1)
	return &serialize.Encoding{
		Format: format,
		Body:   []byte(l.Body),
	}
}

// Encoding2 implements the Bitmessage interface and represents a
// MsgMsg or MsgBroadcast with encoding type 2. It also implements the
type Encoding2 struct {
	Subject string
	Body    string
}

// Encoding returns the encoding format of the bitmessage.
func (l *Encoding2) Encoding() uint64 {
	return 2
}

// Message returns the raw form of the object payload.
func (l *Encoding2) Message() []byte {
	return []byte(fmt.Sprintf("Subject:%s\nBody:%s", l.Subject, l.Body))
}

// ReadMessage reads the object payload and incorporates it.
func (l *Encoding2) ReadMessage(msg []byte) error {
	matches := encoding2Regex.FindStringSubmatch(string(msg))
	if len(matches) < 3 {
		return errors.New("Invalid format")
	}
	l.Subject = matches[1]
	l.Body = matches[2]
	return nil
}

// ToProtobuf encodes the message in a protobuf format.
func (l *Encoding2) ToProtobuf() *serialize.Encoding {
	format := uint64(2)
	return &serialize.Encoding{
		Format:  format,
		Subject: []byte(l.Subject),
		Body:    []byte(l.Body),
	}
}
