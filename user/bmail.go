package user

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/rand"
	"time"

	"github.com/DanielKrawisz/bmagent/user/email"
	"github.com/DanielKrawisz/bmutil/cipher"
	"github.com/DanielKrawisz/bmutil/format"
	"github.com/DanielKrawisz/bmutil/format/serialize"
	"github.com/DanielKrawisz/bmutil/identity"
	"github.com/DanielKrawisz/bmutil/pow"
	"github.com/DanielKrawisz/bmutil/wire"
	"github.com/DanielKrawisz/bmutil/wire/obj"
	"github.com/golang/protobuf/proto"
	"github.com/jordwest/imap-server/types"
)

// bmail is a representation of a bitmessage that can contain a hidden
// field which stores the message as an Object
type bmail struct {
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

// generateBroadcast generates a wire.MsgBroadcast from a Bitmessage.
func (m *bmail) generateBroadcast(from *identity.Private, expiry time.Duration) (obj.Object, *obj.PubKeyData, error) {
	pkd := from.ToPubKeyData()

	var tag wire.ShaHash
	copy(tag[:], from.Address.Tag())

	data := &cipher.Bitmessage{
		FromStreamNumber:   from.Address.Stream,
		FromAddressVersion: from.Address.Version,
		SigningKey:         pkd.VerificationKey,
		EncryptionKey:      pkd.EncryptionKey,
		Pow:                pkd.Pow,
		Content:            m.b.Content,
	}

	var broadcast *cipher.Broadcast
	var err error
	switch from.Address.Version {
	case 2:
		fallthrough
	case 3:
		broadcast, err = cipher.CreateTaglessBroadcast(time.Now().Add(expiry), data, from)
	case 4:
		broadcast, err = cipher.CreateTaggedBroadcast(time.Now().Add(expiry), data, &tag, from)
	default:
		return nil, nil, errors.New("Unknown from address version")
	}

	if err != nil {
		return nil, nil, err
	}

	return broadcast.Object(), pkd, nil
}

// generateMsg generates a cipher.Message from a Bitmessage.
func (m *bmail) generateMessage(from *identity.Private, to *identity.Public, expiry time.Duration) (obj.Object, *obj.PubKeyData, error) {
	// Check for
	if m.b.Ack == nil && m.b.State.AckExpected {
		return nil, nil, email.ErrAckMissing
	}

	pkd := from.ToPubKeyData()

	var destination wire.RipeHash
	copy(destination[:], to.Address.Ripe[:])

	data := &cipher.Bitmessage{
		FromStreamNumber:   from.Address.Stream,
		FromAddressVersion: from.Address.Version,
		Destination:        &destination,
		SigningKey:         pkd.VerificationKey,
		EncryptionKey:      pkd.EncryptionKey,
		Pow:                pkd.Pow,
		Content:            m.b.Content,
	}

	message, err := cipher.SignAndEncryptMessage(time.Now().Add(expiry), from.Address.Stream, data, m.b.Ack, from, to)
	if err != nil {
		return nil, nil, err
	}

	return message.Object(), pkd, nil
}

// GenerateObject generates the wire.MsgObject form of the message.
func (m *bmail) generateObject(s ServerOps) (object obj.Object, data *obj.PubKeyData, genErr error) {

	fromAddr, err := email.ToBm(m.b.From)
	if err != nil {
		return nil, nil, err
	}
	fromID := s.GetPrivateID(fromAddr)
	if fromID == nil {
		email.SMTPLog.Error("GenerateObject: no private id known ")
		return nil, nil, ErrMissingPrivateID
	}
	from := &(fromID.Private)

	// If a wire.Object already exists, just use that.
	if m.object != nil {
		return m.object, from.ToPubKeyData(), nil
	}

	email.SMTPLog.Debug("GenerateObject: about to serialize bmsg from " + m.b.From + " to " + m.b.To)
	if m.b.To == "broadcast@bm.agent" {
		object, data, genErr = m.generateBroadcast(from, s.GetObjectExpiry(wire.ObjectTypeBroadcast))
	} else {
		bmTo, err := email.ToBm(m.b.To)
		if err != nil {
			return nil, nil, err
		}
		to, err := s.GetOrRequestPublicID(bmTo)
		if err != nil {
			email.SMTPLog.Error("GenerateObject: results of lookup public identity: ", to, ", ", err)
			return nil, nil, err
		}
		// Indicates that a pubkey request was sent.
		if to == nil {
			m.b.State.PubkeyRequestOutstanding = true
			return nil, nil, email.ErrGetPubKeySent
		}

		id := s.GetPrivateID(bmTo)
		if id != nil {
			m.b.OfChannel = id.IsChan
			// We're sending to ourselves/chan so don't bother with ack.
			m.b.State.AckExpected = false
		}

		object, data, genErr = m.generateMessage(from, to, s.GetObjectExpiry(wire.ObjectTypeMsg))
	}

	if genErr != nil {
		email.SMTPLog.Error("GenerateObject: ", genErr)
		return nil, nil, genErr
	}
	m.object = object
	return object, data, nil
}

// GenerateAck creates an Ack message
func (m *bmail) generateAck(s ServerOps) (ack obj.Object, powData *pow.Data, err error) {
	// If this is a broadcast message, no ack is expected.
	if m.b.To == "broadcast@bm.agent" {
		return nil, nil, errors.New("No acks on broadcast messages.")
	}

	// Get our private key.
	fromAddr, err := email.ToBm(m.b.From)
	if err != nil {
		return nil, nil, err
	}
	from := s.GetPrivateID(fromAddr)

	addr := from.ToPublic().Address

	// Add the message, which is a random number.
	buf := new(bytes.Buffer)
	num := rand.Int31()
	err = binary.Write(buf, binary.LittleEndian, num)
	if err != nil {
		return nil, nil, err
	}

	// We don't save the message because it still needs POW done on it.
	return wire.NewMsgObject(
		wire.NewObjectHeader(0,
			time.Now().Add(s.GetObjectExpiry(wire.ObjectTypeMsg)),
			wire.ObjectTypeMsg,
			obj.MessageVersion,
			addr.Stream,
		), buf.Bytes()), &from.Data, nil
}
