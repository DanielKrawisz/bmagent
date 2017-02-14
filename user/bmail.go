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

// decodeBitmessage takes the protobuf encoding of a Bitmessage and converts it
// back to a Bitmessage.
func decodeBitmessage(data []byte) (*email.Bmail, obj.Object, error) {
	msg := &serialize.Message{}
	err := proto.Unmarshal(data, msg)
	if err != nil {
		return nil, nil, err
	}

	var q format.Encoding
	switch msg.Encoding.Format {
	case 1:
		r := &format.Encoding1{}

		if msg.Encoding.Body == nil {
			return nil, nil, errors.New("Body required in encoding format 1")
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
			return nil, nil, errors.New("Body required in encoding format 2")
		}
		r.Body = string(msg.Encoding.Body)

		q = r
	default:
		return nil, nil, errors.New("Unsupported encoding")
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
			return nil, nil, err
		}

		l.ImapData = &email.ImapData{
			Flags:        types.Flags(msg.ImapData.Flags),
			TimeReceived: timeReceived,
		}
	}

	if msg.State != nil {
		lastSend, err := time.Parse(email.DateFormat, msg.State.LastSend)
		if err != nil {
			return nil, nil, err
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

	var o obj.Object
	if msg.Object != nil {
		o, err = obj.ReadObject(msg.Object)
		if err != nil {
			return nil, nil, err
		}
	}

	return l, o, nil
}

// serializeBitmessage encodes the message in a protobuf format.
func serializeBitmessage(m *email.Bmail, o obj.Object) ([]byte, error) {
	expr := m.Expiration.Format(email.DateFormat)

	var imapData *serialize.ImapData
	if m.ImapData != nil {
		t := m.ImapData.TimeReceived.Format(email.DateFormat)

		imapData = &serialize.ImapData{
			TimeReceived: t,
			Flags:        int32(m.ImapData.Flags),
		}
	}

	var object []byte
	if o != nil {
		object = wire.Encode(o)
	}

	var state *serialize.MessageState
	if m.State != nil {
		lastsend := m.State.LastSend.Format(email.DateFormat)
		state = &serialize.MessageState{
			SendTries:       m.State.SendTries,
			AckExpected:     m.State.AckExpected,
			AckReceived:     m.State.AckReceived,
			PubkeyRequested: m.State.PubkeyRequestOutstanding,
			LastSend:        lastsend,
			Received:        m.State.Received,
		}
	}

	email.SMTPLog.Trace("Serializing Bitmessage from " + m.From + " to " + m.To)

	encode := &serialize.Message{
		From:       m.From,
		To:         m.To,
		OfChannel:  m.OfChannel,
		Expiration: expr,
		Ack:        m.Ack,
		ImapData:   imapData,
		Encoding:   m.Content.ToProtobuf(),
		Object:     object,
		State:      state,
	}

	data, err := proto.Marshal(encode)
	if err != nil {
		return nil, err
	}
	return data, nil
}
