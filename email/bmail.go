// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/DanielKrawisz/bmutil"
	"github.com/DanielKrawisz/bmutil/cipher"
	"github.com/DanielKrawisz/bmutil/format"
	"github.com/DanielKrawisz/bmutil/format/serialize"
	"github.com/DanielKrawisz/bmutil/identity"
	"github.com/DanielKrawisz/bmutil/pow"
	"github.com/DanielKrawisz/bmutil/wire"
	"github.com/DanielKrawisz/bmutil/wire/obj"
	"github.com/golang/protobuf/proto"
	"github.com/jordwest/imap-server/types"
	"github.com/mailhog/data"
)

const (
	// dateFormat is the format for encoding dates.
	dateFormat = "Mon Jan 2 15:04:05 -0700 MST 2006"

	// Pattern used for matching Bitmessage addresses.
	bmAddrPattern = "BM-[123456789abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ]+"

	commandPattern = "[a-z]+"
)

var (
	// ErrInvalidEmail is the error returned by EmailToBM when the e-mail
	// address is invalid and a valid Bitmessage address cannot be extracted
	// from it.
	ErrInvalidEmail = fmt.Errorf("From address must be of the form %s.",
		emailRegexString)

	// ErrGetPubKeySent is the error returned by GenerateObject when a
	// a message could not be written as an object because the necessary
	// PubKey was missing. However, a GetPubKey request was successfully
	// sent, so we should receive it eventually.
	ErrGetPubKeySent = fmt.Errorf("GetPubKey request sent")

	// ErrAckMissing is the error returned by GenerateObject when a
	// message could not be written as an object because an ack is required
	// but missing. Therefore it is necessary to generate the ack and
	// do POW on it before proceeding.
	ErrAckMissing = fmt.Errorf("Ack missing")

	bitmessageRegex = regexp.MustCompile(fmt.Sprintf("^%s$", bmAddrPattern))

	emailRegexString = fmt.Sprintf("^%s@bm\\.addr$", bmAddrPattern)

	// emailRegex is used for extracting Bitmessage address from an e-mail
	// address.
	emailRegex = regexp.MustCompile(emailRegexString)

	commandRegexString = fmt.Sprintf("^%s@bm\\.agent$", commandPattern)

	// commandRegex is used for detecting an email intended as a
	// command to bmagent.
	commandRegex = regexp.MustCompile(commandRegexString)
)

// bmToEmail converts a Bitmessage address to an e-mail address.
func bmToEmail(bmAddr string) string {
	return fmt.Sprintf("%s@bm.addr", bmAddr)
}

// emailToBM extracts a Bitmessage address from an e-mail address.
func emailToBM(emailAddr string) (string, error) {
	addr, err := mail.ParseAddress(emailAddr)
	if err != nil {
		return "", err
	}

	if !emailRegex.Match([]byte(addr.Address)) {
		return "", ErrInvalidEmail
	}

	bm := strings.Split(addr.Address, "@")[0]

	if _, err := bmutil.DecodeAddress(bm); err != nil {
		return "", ErrInvalidEmail
	}
	return bm, nil
}

// ImapData provides a Bitmessage with extra information to make it
// compatible with imap.
type ImapData struct {
	UID            uint64
	SequenceNumber uint32
	Flags          types.Flags
	TimeReceived   time.Time
	Mailbox        Mailbox
}

// MessageState contains the state of the message as maintained by bmclient.
type MessageState struct {
	// Whether a pubkey request is pending for this message.
	PubkeyRequestOutstanding bool
	// The number of times that the message was sent.
	SendTries uint32
	// The last send attempt for the message.
	LastSend time.Time
	// Whether an ack is expected for this message.
	AckExpected bool
	// Whether an ack has been received.
	AckReceived bool
	// Whether the message was received over the bitmessage network.
	Received bool
}

// Bmail represents an email compatible with a bitmessage format
// (msg or broadcast). If To is empty, then it is a broadcast.
type Bmail struct {
	From       string
	To         string
	OfChannel  bool // Whether the message was sent to/received from a channel.
	Expiration time.Time
	powData    *pow.Data
	Ack        []byte
	Content    format.Encoding
	ImapData   *ImapData
	// The encoded form of the message as a bitmessage object. Required
	// for messages that are waiting to be sent or have pow done on them.
	object obj.Object
	state  *MessageState
}

// Serialize encodes the message in a protobuf format.
func (m *Bmail) Serialize() ([]byte, error) {
	expr := m.Expiration.Format(dateFormat)

	var imapData *serialize.ImapData
	if m.ImapData != nil {
		t := m.ImapData.TimeReceived.Format(dateFormat)

		imapData = &serialize.ImapData{
			TimeReceived: t,
			Flags:        int32(m.ImapData.Flags),
		}
	}

	var object []byte
	if m.object != nil {
		object = wire.Encode(m.object)
	}

	var state *serialize.MessageState
	if m.state != nil {
		lastsend := m.state.LastSend.Format(dateFormat)
		state = &serialize.MessageState{
			SendTries:       m.state.SendTries,
			AckExpected:     m.state.AckExpected,
			AckReceived:     m.state.AckReceived,
			PubkeyRequested: m.state.PubkeyRequestOutstanding,
			LastSend:        lastsend,
			Received:        m.state.Received,
		}
	}

	smtpLog.Trace("Serializing Bitmessage from " + m.From + " to " + m.To)

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

// generateBroadcast generates a wire.MsgBroadcast from a Bitmessage.
func (m *Bmail) generateBroadcast(from *identity.Private, expiry time.Duration) (obj.Object, *pow.Data, error) {
	// TODO make a separate function in bmutil that does this.
	var signingKey, encKey wire.PubKey
	var tag wire.ShaHash
	sk := from.SigningKey.PubKey().SerializeUncompressed()[1:]
	ek := from.EncryptionKey.PubKey().SerializeUncompressed()[1:]
	t := from.Address.Tag()
	copy(signingKey[:], sk)
	copy(encKey[:], ek)
	copy(tag[:], t)

	powData := &pow.Data{
		NonceTrialsPerByte: from.NonceTrialsPerByte,
		ExtraBytes:         from.ExtraBytes,
	}

	data := &cipher.Bitmessage{
		FromStreamNumber:   from.Address.Stream,
		FromAddressVersion: from.Address.Version,
		SigningKey:         &signingKey,
		EncryptionKey:      &encKey,
		Pow:                powData,
		Content:            m.Content,
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

	return broadcast.Object(), powData, nil
}

// generateMsg generates a cipher.Message from a Bitmessage.
func (m *Bmail) generateMessage(from *identity.Private, to *identity.Public, expiry time.Duration) (obj.Object, *pow.Data, error) {
	// Check for
	if m.Ack == nil && m.state.AckExpected {
		return nil, nil, ErrAckMissing
	}

	var signingKey, encKey wire.PubKey
	var destination wire.RipeHash
	sk := from.SigningKey.PubKey().SerializeUncompressed()[1:]
	ek := from.EncryptionKey.PubKey().SerializeUncompressed()[1:]
	copy(signingKey[:], sk)
	copy(encKey[:], ek)
	copy(destination[:], to.Address.Ripe[:])

	powData := &pow.Data{
		NonceTrialsPerByte: from.NonceTrialsPerByte,
		ExtraBytes:         from.ExtraBytes,
	}

	data := &cipher.Bitmessage{
		FromStreamNumber:   from.Address.Stream,
		FromAddressVersion: from.Address.Version,
		Destination:        &destination,
		SigningKey:         &signingKey,
		EncryptionKey:      &encKey,
		Pow:                powData,
		Content:            m.Content,
	}

	message, err := cipher.SignAndEncryptMessage(time.Now().Add(expiry), from.Address.Stream, data, m.Ack, from, to)
	if err != nil {
		return nil, nil, err
	}

	return message.Object(), powData, nil
}

// GenerateObject generates the wire.MsgObject form of the message.
func (m *Bmail) GenerateObject(s ServerOps) (object obj.Object,
	powData *pow.Data, genErr error) {

	// If a wire.Object already exists, just use that.
	if m.object != nil {
		return m.object, m.powData, nil
	}

	smtpLog.Debug("GenerateObject: about to serialize bmsg from " + m.From + " to " + m.To)
	fromAddr, err := emailToBM(m.From)
	if err != nil {
		return nil, nil, err
	}
	from := s.GetPrivateID(fromAddr)
	if from == nil {
		smtpLog.Error("GenerateObject: no private id known ")
		return nil, nil, errors.New("Private id not found")
	}

	if m.To == "broadcast@bm.agent" {
		object, powData, genErr = m.generateBroadcast(&(from.Private), s.GetObjectExpiry(wire.ObjectTypeBroadcast))
	} else {
		bmTo, err := emailToBM(m.To)
		if err != nil {
			return nil, nil, err
		}
		to, err := s.GetOrRequestPublicID(bmTo)
		if err != nil {
			smtpLog.Error("GenerateObject: results of lookup public identity: ", to, ", ", err)
			return nil, nil, err
		}
		// Indicates that a pubkey request was sent.
		if to == nil {
			m.state.PubkeyRequestOutstanding = true
			return nil, nil, ErrGetPubKeySent
		}

		id := s.GetPrivateID(bmTo)
		if id != nil {
			m.OfChannel = id.IsChan
			// We're sending to ourselves/chan so don't bother with ack.
			m.state.AckExpected = false
		}

		object, powData, genErr = m.generateMessage(&(from.Private), to, s.GetObjectExpiry(wire.ObjectTypeMsg))
	}

	if genErr != nil {
		smtpLog.Error("GenerateObject: ", genErr)
		return nil, nil, genErr
	}
	m.object = object
	return object, powData, nil
}

func (m *Bmail) generateAck(s ServerOps) (ack obj.Object, powData *pow.Data, err error) {
	// If this is a broadcast message, no ack is expected.
	if m.To == "broadcast@bm.agent" {
		return nil, nil, errors.New("No acks on broadcast messages.")
	}

	// Get our private key.
	fromAddr, err := emailToBM(m.From)
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

// MsgRead creates a Bitmessage object from an unencrypted wire.MsgMsg.
func MsgRead(msg *cipher.Message, toAddress string, ofChan bool) (*Bmail, error) {
	data := msg.Bitmessage()
	object := msg.Object()
	header := object.Header()

	sign, _ := data.SigningKey.ToBtcec()
	encr, _ := data.EncryptionKey.ToBtcec()
	from := identity.NewPublic(sign, encr, data.Pow, data.FromAddressVersion, data.FromStreamNumber)
	fromAddress, err := from.Address.Encode()
	if err != nil {
		return nil, err
	}

	return &Bmail{
		From:       bmToEmail(fromAddress),
		To:         bmToEmail(toAddress),
		Expiration: header.Expiration(),
		Ack:        msg.Ack(),
		OfChannel:  ofChan,
		Content:    data.Content,
		object:     object.MsgObject(),
	}, nil
}

// BroadcastRead creates a Bitmessage object from an unencrypted
// wire.MsgBroadcast.
func BroadcastRead(msg *cipher.Broadcast) (*Bmail, error) {
	header := msg.Object().Header()
	data := msg.Bitmessage()

	sign, _ := data.SigningKey.ToBtcec()
	encr, _ := data.EncryptionKey.ToBtcec()
	from := identity.NewPublic(sign, encr, data.Pow, data.FromAddressVersion, data.FromStreamNumber)
	fromAddress, err := from.Address.Encode()
	if err != nil {
		return nil, err
	}

	return &Bmail{
		From:       "broadcast@bm.agent",
		To:         bmToEmail(fromAddress),
		Expiration: header.Expiration(),
		Content:    data.Content,
	}, nil
}

// DecodeBitmessage takes the protobuf encoding of a Bitmessage and converts it
// back to a Bitmessage.
func DecodeBitmessage(data []byte) (*Bmail, error) {
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

	l := &Bmail{}

	if msg.Expiration != "" {
		expr, _ := time.Parse(dateFormat, msg.Expiration)
		l.Expiration = expr
	}

	l.From = msg.From
	l.To = msg.To
	l.OfChannel = msg.OfChannel
	l.Ack = msg.Ack
	l.Content = q
	if msg.Object != nil {
		l.object, err = obj.ReadObject(msg.Object)
		if err != nil {
			return nil, err
		}
	}

	if msg.ImapData != nil {
		timeReceived, err := time.Parse(dateFormat, msg.ImapData.TimeReceived)
		if err != nil {
			return nil, err
		}

		l.ImapData = &ImapData{
			Flags:        types.Flags(msg.ImapData.Flags),
			TimeReceived: timeReceived,
		}
	}

	if msg.State != nil {
		lastSend, err := time.Parse(dateFormat, msg.State.LastSend)
		if err != nil {
			return nil, err
		}

		l.state = &MessageState{
			PubkeyRequestOutstanding: msg.State.PubkeyRequested,
			SendTries:                msg.State.SendTries,
			LastSend:                 lastSend,
			AckExpected:              msg.State.AckExpected,
			AckReceived:              msg.State.AckReceived,
			Received:                 msg.State.Received,
		}
	}

	return l, nil
}

// ToEmail converts a Bitmessage into an IMAPEmail.
func (m *Bmail) ToEmail() (*IMAPEmail, error) {
	var payload *format.Encoding2
	switch m := m.Content.(type) {
	// Only encoding 2 is considered to be compatible with email.
	// In the future, there may be more encodings also compatible with email.
	case *format.Encoding2:
		payload = m
	default:
		return nil, errors.New("Wrong format")
	}

	if m.ImapData == nil {
		return nil, errors.New("No IMAP data")
	}

	headers := make(map[string][]string)

	headers["Subject"] = []string{payload.Subject}

	headers["From"] = []string{m.From}

	if m.To == "" {
		headers["To"] = []string{"broadcast@bm.agent"}
	} else {
		headers["To"] = []string{m.To}
	}

	headers["Date"] = []string{m.ImapData.TimeReceived.Format(dateFormat)}
	headers["Expires"] = []string{m.Expiration.Format(dateFormat)}
	if m.OfChannel {
		headers["Reply-To"] = []string{m.To}
	}
	headers["Content-Type"] = []string{`text/plain; charset="UTF-8"`}
	headers["Content-Transfer-Encoding"] = []string{"8bit"}

	content := &data.Content{
		Headers: headers,
		Body:    payload.Body,
	}

	email := &IMAPEmail{
		ImapSequenceNumber: m.ImapData.SequenceNumber,
		ImapUID:            m.ImapData.UID,
		ImapFlags:          m.ImapData.Flags,
		Date:               m.ImapData.TimeReceived,
		Mailbox:            m.ImapData.Mailbox,
		Content:            content,
	}

	// Calculate the size of the message.
	content.Size = len(fmt.Sprintf("%s\r\n", email.Header())) + len(payload.Body)

	return email, nil
}

// NewBitmessageFromSMTP takes an SMTP e-mail and turns it into a Bitmessage.
func NewBitmessageFromSMTP(smtp *data.Content) (*Bmail, error) {
	header := smtp.Headers

	// Check that To and From are set.
	toList, ok := header["To"]
	if !ok {
		return nil, errors.New("Invalid headers: To field is required")
	}

	if len(toList) != 1 {
		return nil, errors.New("Invalid headers: only one To field is allowed.")
	}

	fromList, ok := header["From"]
	if !ok {
		return nil, errors.New("Invalid headers: From field is required")
	}

	if len(fromList) != 1 {
		return nil, errors.New("Invalid headers: only one From field is allowed.")
	}

	var from, to string

	if !(validateEmail(fromList[0]) || validateEmail(toList[0])) {
		return nil, ErrInvalidEmail
	}

	// No errors because this must have succeeded when the
	// address was validated above.
	fromAddr, _ := mail.ParseAddress(fromList[0])
	toAddr, _ := mail.ParseAddress(toList[0])
	from = fromAddr.Address
	to = toAddr.Address

	// If CC or BCC are set, give an error because these headers cannot
	// be made to work as the user would expect with the Bitmessage protocol
	// as it is currently defined.
	cc, okCc := header["Cc"]
	bcc, okBcc := header["Bcc"]
	if (okCc && len(cc) > 0) || (okBcc && len(bcc) > 0) {
		return nil, errors.New("Invalid headers: do not use CC or BCC with Bitmessage")
	}

	// Expires is a rarely-used header that is relevant to Bitmessage.
	// If it is set, use it to generate the expire time of the message.
	// Otherwise, use the default.
	var expiration time.Time
	if expireStr, ok := header["Expires"]; ok {
		exp, err := time.Parse(dateFormat, expireStr[0])
		if err != nil {
			return nil, err
		}
		expiration = exp
	}

	var subject string
	if subj, ok := header["Subject"]; ok {
		subject = subj[0]
	} else {
		subject = ""
	}

	body, err := getSMTPBody(smtp)
	if err != nil {
		return nil, err
	}

	return &Bmail{
		From:       from,
		To:         to,
		Expiration: expiration,
		Ack:        nil,
		Content: &format.Encoding2{
			Subject: subject,
			Body:    body,
		},
		state: &MessageState{
			// false if broadcast; Code for setting it false if sending to
			// channel/self is in GenerateObject.
			AckExpected: to != "",
		},
	}, nil
}

// NewBitmessageDraftFromSMTP takes an SMTP e-mail and turns it into a Bitmessage,
// but is less strict than NewBitmessageFromSMTP in how it checks the email.
func NewBitmessageDraftFromSMTP(smtp *data.Content) (*Bmail, error) {
	header := smtp.Headers

	// Check that To and From are set.
	var to, from string

	toList, ok := header["To"]
	if !ok || len(toList) != 1 {
		to = ""
	} else if len(toList) != 1 {
		to = toList[0]
	}

	fromList, ok := header["From"]
	if !ok || len(fromList) != 1 {
		from = ""
	} else {
		from = fromList[0]
	}

	// Expires is a rarely-used header that is relevant to Bitmessage.
	// If it is set, use it to generate the expire time of the message.
	// Otherwise, use the default.
	var expiration time.Time
	if expireStr, ok := header["Expires"]; ok {
		exp, err := time.Parse(dateFormat, expireStr[0])
		if err != nil {
			return nil, err
		}
		expiration = exp
	}

	var subject string
	if subj, ok := header["Subject"]; ok {
		subject = subj[0]
	} else {
		subject = ""
	}

	body, err := getSMTPBody(smtp)
	if err != nil {
		return nil, err
	}

	return &Bmail{
		From:       from,
		To:         to,
		Expiration: expiration,
		Ack:        nil,
		Content: &format.Encoding2{
			Subject: subject,
			Body:    body,
		},
		state: &MessageState{
			// false if broadcast; Code for setting it false if sending to
			// channel/self is in GenerateObject.
			AckExpected: to != "",
		},
	}, nil
}
