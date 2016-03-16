// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package format

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/DanielKrawisz/bmagent/message/serialize"
)

var encoding2Regex = regexp.MustCompile(`^Subject:(.*)\nBody:((?s).*)`)

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
	return &serialize.Encoding{
		Format: l.Encoding(),
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
	return &serialize.Encoding{
		Format:  l.Encoding(),
		Subject: []byte(l.Subject),
		Body:    []byte(l.Body),
	}
}

// DecodeObjectPayload takes an encoding format code and an object payload and
// returns it as an Encoding object.
func DecodeObjectPayload(encoding uint64, msg []byte) (Encoding, error) {
	var q Encoding
	switch encoding {
	case 1:
		q = &Encoding1{}
	case 2:
		q = &Encoding2{}
	default:
		return nil, errors.New("Unsupported encoding")
	}
	err := q.ReadMessage(msg)
	if err != nil {
		return nil, err
	}
	return q, nil
}
