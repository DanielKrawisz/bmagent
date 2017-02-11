package user

import (
	"errors"
	"time"

	"github.com/DanielKrawisz/bmagent/user/email"
	"github.com/DanielKrawisz/bmutil/format"
	"github.com/DanielKrawisz/bmutil/format/serialize"
	"github.com/DanielKrawisz/bmutil/identity"
	"github.com/DanielKrawisz/bmutil/wire"
	"github.com/DanielKrawisz/bmutil/wire/obj"
	"github.com/golang/protobuf/proto"
	"github.com/jordwest/imap-server/types"
)

// Broadcast represents a broadcast message, which doesn't require a public id.
var Broadcast = &identity.Public{}

// bmail is a representation of a bitmessage that can contain a hidden
// field which stores the message as an Object
type bmail struct {
	// The
	b *email.Bmail

	// The encoded form of the message as a bitmessage object. Required
	// for messages that are waiting to be sent or have pow done on them.
	object obj.Object
}

// decodeBitmessage takes the protobuf encoding of a Bitmessage and converts it
// back to a Bitmessage.
func decodeBitmessage(data []byte) (*bmail, error) {
	msg := &serialize.Message{}
	err := proto.Unmarshal(data, msg)
	if err != nil {
		return nil, err
	}

	var q format.Encoding
	switch msg.Encoding.Format {
	case 1:
		r := &format.Encoding1{}

		if msg.Encoding.Body == nil {
			return nil, errors.New("Body required in encoding format 1")
		}
		r.Body = string(msg.Encoding.Body)

		q = r
	case 2:
		r := &format.Encoding2{}

		if msg.Encoding.Subject == nil {
			r.Subject = ""
		} else {
			r.Subject = string(msg.Encoding.Subject)
		}

		if msg.Encoding.Body == nil {
			return nil, errors.New("Body required in encoding format 2")
		}
		r.Body = string(msg.Encoding.Body)

		q = r
	default:
		return nil, errors.New("Unsupported encoding")
	}

	l := &email.Bmail{}

	if msg.Expiration != "" {
		expr, _ := time.Parse(email.DateFormat, msg.Expiration)
		l.Expiration = expr
	}

	l.From = msg.From
	l.To = msg.To
	l.OfChannel = msg.OfChannel
	l.Content = q
	l.Ack = msg.Ack

	if msg.ImapData != nil {
		timeReceived, err := time.Parse(email.DateFormat, msg.ImapData.TimeReceived)
		if err != nil {
			return nil, err
		}

		l.ImapData = &email.ImapData{
			Flags:        types.Flags(msg.ImapData.Flags),
			TimeReceived: timeReceived,
		}
	}

	if msg.State != nil {
		lastSend, err := time.Parse(email.DateFormat, msg.State.LastSend)
		if err != nil {
			return nil, err
		}

		l.State = &email.MessageState{
			PubkeyRequestOutstanding: msg.State.PubkeyRequested,
			SendTries:                msg.State.SendTries,
			LastSend:                 lastSend,
			AckExpected:              msg.State.AckExpected,
			AckReceived:              msg.State.AckReceived,
			Received:                 msg.State.Received,
		}
	}

	b := &bmail{
		b: l,
	}

	if msg.Object != nil {
		b.object, err = obj.ReadObject(msg.Object)
		if err != nil {
			return nil, err
		}
	}

	return b, nil
}

// serialize encodes the message in a protobuf format.
func (m *bmail) serialize() ([]byte, error) {
	expr := m.b.Expiration.Format(email.DateFormat)

	var imapData *serialize.ImapData
	if m.b.ImapData != nil {
		t := m.b.ImapData.TimeReceived.Format(email.DateFormat)

		imapData = &serialize.ImapData{
			TimeReceived: t,
			Flags:        int32(m.b.ImapData.Flags),
		}
	}

	var object []byte
	if m.object != nil {
		object = wire.Encode(m.object)
	}

	var state *serialize.MessageState
	if m.b.State != nil {
		lastsend := m.b.State.LastSend.Format(email.DateFormat)
		state = &serialize.MessageState{
			SendTries:       m.b.State.SendTries,
			AckExpected:     m.b.State.AckExpected,
			AckReceived:     m.b.State.AckReceived,
			PubkeyRequested: m.b.State.PubkeyRequestOutstanding,
			LastSend:        lastsend,
			Received:        m.b.State.Received,
		}
	}

	email.SMTPLog.Trace("Serializing Bitmessage from " + m.b.From + " to " + m.b.To)

	encode := &serialize.Message{
		From:       m.b.From,
		To:         m.b.To,
		OfChannel:  m.b.OfChannel,
		Expiration: expr,
		Ack:        m.b.Ack,
		ImapData:   imapData,
		Encoding:   m.b.Content.ToProtobuf(),
		Object:     object,
		State:      state,
	}

	data, err := proto.Marshal(encode)
	if err != nil {
		return nil, err
	}
	return data, nil
}
