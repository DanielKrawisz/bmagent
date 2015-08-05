// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

import (
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/jordwest/imap-server/types"
	"github.com/mailhog/data"
	"github.com/DanielKrawisz/bmagent/message/format"
	"github.com/DanielKrawisz/bmagent/message/serialize"
	"github.com/DanielKrawisz/bmutil"
	"github.com/DanielKrawisz/bmutil/cipher"
	"github.com/DanielKrawisz/bmutil/identity"
	"github.com/DanielKrawisz/bmutil/pow"
	"github.com/DanielKrawisz/bmutil/wire"
)

const (
	// dateFormat is the format for encoding dates.
	dateFormat = "Mon Jan 2 15:04:05 -0700 MST 2006"

	// Pattern used for matching Bitmessage addresses.
	bmAddrPattern = "BM-[123456789abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ]+"
)

var (
	// ErrInvalidEmail is the error returned by EmailToBM when the e-mail
	// address is invalid and a valid Bitmessage address cannot be extracted
	// from it.
	ErrInvalidEmail = errors.New("Invalid Bitmessage address")
	
	emailRegexString = fmt.Sprintf("%s@bm\\.addr", bmAddrPattern)

	// emailRegex is used for extracting Bitmessage address from an e-mail
	// address.
	emailRegex = regexp.MustCompile(emailRegexString)
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

	matches := emailRegex.FindStringSubmatch(addr.Address)
	if len(matches) < 2 {
		return "", ErrInvalidEmail
	}

	if _, err := bmutil.DecodeAddress(matches[1]); err != nil {
		return "", ErrInvalidEmail
	}
	return matches[1], nil
}

// IMAPData provides a Bitmessage with extra information to make it
// compatible with imap.
type IMAPData struct {
	UID            uint64
	SequenceNumber uint32
	Flags          types.Flags
	TimeReceived   time.Time
	Mailbox        *Mailbox
}

// MessageState contains the state of the message as maintained by bmclient.
type MessageState struct {
	// Whether a pubkey request is pending for this message.
	PubkeyRequested bool
	// The index of a pow computation for this message.
	PowIndex uint64
	// The index of a pow computation for the message ack.
	AckPowIndex uint64
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

// Bitmessage represents a message compatible with a bitmessage format
// (msg or broadcast). If To is empty, then it is a broadcast.
type Bitmessage struct {
	From       string
	To         string
	OfChannel  bool // Whether the message was sent to/received from a channel.
	Expiration time.Time
	Ack        []byte
	Message    format.Encoding
	ImapData   *IMAPData
	// The encoded form of the message as a bitmessage object. Required
	// for messages that are waiting to be sent or have pow done on them.
	object *wire.MsgObject
	state  *MessageState
}

// Serialize encodes the message in a protobuf format.
func (m *Bitmessage) Serialize() ([]byte, error) {
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
		object = wire.EncodeMessage(m.object)
	}

	var state *serialize.MessageState
	if m.state != nil {
		lastsend := m.state.LastSend.Format(dateFormat)
		state = &serialize.MessageState{
			SendTries:       m.state.SendTries,
			AckExpected:     m.state.AckExpected,
			AckReceived:     m.state.AckReceived,
			PubkeyRequested: m.state.PubkeyRequested,
			PowIndex:        m.state.PowIndex,
			AckPowIndex:     m.state.AckPowIndex,
			LastSend:        lastsend,
			Received:        m.state.Received,
		}
	}

	encode := &serialize.Message{
		From:       m.From,
		To:         m.To,
		OfChannel:  m.OfChannel,
		Expiration: expr,
		Ack:        m.Ack,
		ImapData:   imapData,
		Encoding:   m.Message.ToProtobuf(),
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
func (m *Bitmessage) generateBroadcast(from *identity.Private, expiry time.Duration) (*wire.MsgObject, uint64, uint64, error) {
	// TODO make a separate function in bmutil that does this.
	var signingKey, encKey wire.PubKey
	var tag wire.ShaHash
	sk := from.SigningKey.PubKey().SerializeUncompressed()[1:]
	ek := from.EncryptionKey.PubKey().SerializeUncompressed()[1:]
	t := from.Address.Tag()
	copy(signingKey[:], sk)
	copy(encKey[:], ek)
	copy(tag[:], t)

	var version uint64
	switch from.Address.Version {
	case 2:
		fallthrough
	case 3:
		version = wire.TaglessBroadcastVersion
	case 4:
		version = wire.TagBroadcastVersion
	default:
		return nil, 0, 0, errors.New("Unknown from address version")
	}

	broadcast := &wire.MsgBroadcast{
		ObjectType:         wire.ObjectTypeBroadcast,
		Version:            version,
		ExpiresTime:        time.Now().Add(expiry),
		StreamNumber:       from.Address.Stream,
		FromStreamNumber:   from.Address.Stream,
		FromAddressVersion: from.Address.Version,
		NonceTrials:        from.NonceTrialsPerByte,
		ExtraBytes:         from.ExtraBytes,
		SigningKey:         &signingKey,
		EncryptionKey:      &encKey,

		Encoding: m.Message.Encoding(),
		Message:  m.Message.Message(),
		Tag:      &tag,
	}

	err := cipher.SignAndEncryptBroadcast(broadcast, from)
	if err != nil {
		return nil, 0, 0, err
	}

	obj, err := wire.ToMsgObject(broadcast)
	if err != nil {
		return nil, 0, 0, err
	}
	return obj, from.NonceTrialsPerByte, from.ExtraBytes, nil
}

// generateMsg generates a wire.MsgMsg from a Bitmessage.
func (m *Bitmessage) generateMsg(from *identity.Private, to *identity.Public, expiry time.Duration) (*wire.MsgObject, uint64, uint64, error) {
	// TODO make a separate function in bmutil that does this.
	var signingKey, encKey wire.PubKey
	var destination wire.RipeHash
	sk := from.SigningKey.PubKey().SerializeUncompressed()[1:]
	ek := from.EncryptionKey.PubKey().SerializeUncompressed()[1:]
	copy(signingKey[:], sk)
	copy(encKey[:], ek)
	copy(destination[:], to.Address.Ripe[:])

	message := &wire.MsgMsg{
		ObjectType:         wire.ObjectTypeMsg,
		ExpiresTime:        time.Now().Add(expiry),
		Version:            1,
		StreamNumber:       from.Address.Stream,
		FromStreamNumber:   from.Address.Stream,
		FromAddressVersion: from.Address.Version,
		NonceTrials:        from.NonceTrialsPerByte,
		ExtraBytes:         from.ExtraBytes,
		SigningKey:         &signingKey,
		EncryptionKey:      &encKey,
		Destination:        &destination,

		Encoding: m.Message.Encoding(),
		Message:  m.Message.Message(),
	}

	// TODO generate ack

	err := cipher.SignAndEncryptMsg(message, from, to)
	if err != nil {
		return nil, 0, 0, err
	}

	obj, err := wire.ToMsgObject(message)
	if err != nil {
		return nil, 0, 0, err
	}
	return obj, from.NonceTrialsPerByte, from.ExtraBytes, nil
}

// GenerateObject generates the wire.MsgObject form of the message.
func (m *Bitmessage) GenerateObject(s ServerOps) (object *wire.MsgObject,
	nonceTrials, extraBytes uint64, genErr error) {
	from, err := s.GetPrivateID(m.From)
	if err != nil {
		smtpLog.Error("GenerateObject: Error returned private identity: ", err)
		return nil, 0, 0, err
	}

	if m.To == "" {
		object, nonceTrials, extraBytes, genErr = m.generateBroadcast(&(from.Private), s.GetObjectExpiry(wire.ObjectTypeBroadcast))
	} else {
		to, err := s.GetOrRequestPublicID(m.To)
		if err != nil {
			smtpLog.Error("GenerateObject: results of lookup public identity: ", to, ", ", err)
			return nil, 0, 0, err
		}
		// Indicates that a pubkey request was sent.
		if to == nil {
			m.state.PubkeyRequested = true
			return nil, 0, 0, nil
		}

		id, err := s.GetPrivateID(m.To)
		if err == nil {
			m.OfChannel = id.IsChan
			// We're sending to ourselves/chan so don't bother with ack.
			m.state.AckExpected = false
		}

		object, nonceTrials, extraBytes, genErr = m.generateMsg(&(from.Private), to, s.GetObjectExpiry(wire.ObjectTypeMsg))
	}

	if genErr != nil {
		smtpLog.Error("GenerateObject: ", err)
		return nil, 0, 0, genErr
	}
	m.object = object
	return object, nonceTrials, extraBytes, nil
}

// SubmitPow attempts to submit a message for pow.
func (m *Bitmessage) SubmitPow(s ServerOps) (uint64, error) {
	// Attempt to generate the wire.Object form of the message.
	obj, nonceTrials, extraBytes, err := m.GenerateObject(s)
	if obj == nil {
		smtpLog.Error("SubmitPow: could not generate message. Pubkey request sent? ", err == nil)
		if err == nil {
			return 0, nil
		}
		return 0, err
	}

	// If we were able to generate the object, put it in the pow queue.
	encoded := wire.EncodeMessage(obj)
	q := encoded[8:] // exclude the nonce

	target := pow.CalculateTarget(uint64(len(q)),
		uint64(obj.ExpiresTime.Sub(time.Now()).Seconds()), nonceTrials, extraBytes)
	index, err := s.RunPow(target, q)
	if err != nil {
		return 0, err
	}

	m.state.PowIndex = index

	return index, nil
}

// MsgRead creates a Bitmessage object from an unencrypted wire.MsgMsg.
func MsgRead(msg *wire.MsgMsg, toAddress string, ofChan bool) (*Bitmessage, error) {
	message, err := format.DecodeObjectPayload(msg.Encoding, msg.Message)
	if err != nil {
		return nil, err
	}

	sign, _ := msg.SigningKey.ToBtcec()
	encr, _ := msg.EncryptionKey.ToBtcec()
	from := identity.NewPublic(sign, encr, msg.NonceTrials,
		msg.ExtraBytes, msg.FromAddressVersion, msg.FromStreamNumber)
	fromAddress, err := from.Address.Encode()
	if err != nil {
		return nil, err
	}

	return &Bitmessage{
		From:       fromAddress,
		To:         toAddress,
		Expiration: msg.ExpiresTime,
		Ack:        msg.Ack,
		OfChannel:  ofChan,
		Message:    message,
	}, nil
}

// BroadcastRead creates a Bitmessage object from an unencrypted
// wire.MsgBroadcast.
func BroadcastRead(msg *wire.MsgBroadcast) (*Bitmessage, error) {
	message, err := format.DecodeObjectPayload(msg.Encoding, msg.Message)
	if err != nil {
		return nil, err
	}

	sign, _ := msg.SigningKey.ToBtcec()
	encr, _ := msg.EncryptionKey.ToBtcec()
	from := identity.NewPublic(sign, encr, msg.NonceTrials,
		msg.ExtraBytes, msg.FromAddressVersion, msg.FromStreamNumber)
	fromAddress, err := from.Address.Encode()
	if err != nil {
		return nil, err
	}

	return &Bitmessage{
		From:       fromAddress,
		Expiration: msg.ExpiresTime,
		Message:    message,
	}, nil
}

// DecodeBitmessage takes the protobuf encoding of a Bitmessage and converts it
// back to a Bitmessage.
func DecodeBitmessage(data []byte) (*Bitmessage, error) {
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
			return nil, errors.New("Subject required in encoding format 2")
		}
		r.Subject = string(msg.Encoding.Subject)

		if msg.Encoding.Body == nil {
			return nil, errors.New("Body required in encoding format 2")
		}
		r.Body = string(msg.Encoding.Body)

		q = r
	default:
		return nil, errors.New("Unsupported encoding")
	}

	l := &Bitmessage{}

	if msg.Expiration != "" {
		expr, _ := time.Parse(dateFormat, msg.Expiration)
		l.Expiration = expr
	}

	l.From = msg.From
	l.To = msg.To
	l.OfChannel = msg.OfChannel
	l.Ack = msg.Ack
	l.Message = q
	if msg.Object != nil {
		l.object, err = wire.DecodeMsgObject(msg.Object)
		if err != nil {
			return nil, err
		}
	}

	if msg.ImapData != nil {
		timeReceived, err := time.Parse(dateFormat, msg.ImapData.TimeReceived)
		if err != nil {
			return nil, err
		}

		l.ImapData = &IMAPData{
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
			PubkeyRequested: msg.State.PubkeyRequested,
			PowIndex:        msg.State.PowIndex,
			AckPowIndex:     msg.State.AckPowIndex,
			SendTries:       msg.State.SendTries,
			LastSend:        lastSend,
			AckExpected:     msg.State.AckExpected,
			AckReceived:     msg.State.AckReceived,
			Received:        msg.State.Received,
		}
	}

	return l, nil
}

// ToEmail converts a Bitmessage into an IMAPEmail.
func (m *Bitmessage) ToEmail() (*IMAPEmail, error) {
	var payload *format.Encoding2
	switch m := m.Message.(type) {
	// Only encoding 2 is considered to be compatible with email.
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

	headers["From"] = []string{bmToEmail(m.From)}

	if m.To == "" {
		headers["To"] = []string{BroadcastAddress}
	} else {
		headers["To"] = []string{bmToEmail(m.To)}
	}

	headers["Date"] = []string{m.ImapData.TimeReceived.Format(dateFormat)}
	headers["Expires"] = []string{m.Expiration.Format(dateFormat)}
	if m.OfChannel {
		headers["Reply-To"] = []string{bmToEmail(m.To)}
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
func NewBitmessageFromSMTP(smtp *data.Content) (*Bitmessage, error) {
	header := smtp.Headers

	// Check that To and From are set.
	var to, from string
	toList, ok := header["To"]
	if !ok {
		return nil, errors.New("Invalid headers: To field is required")
	}

	if len(toList) != 1 {
		return nil, errors.New("Invalid headers: only one To field is allowed.")
	}
	to = toList[0]

	fromList, ok := header["From"]
	if !ok {
		return nil, errors.New("Invalid headers: From field is required")
	}

	if len(fromList) != 1 {
		return nil, errors.New("Invalid headers: only one From field is allowed.")
	}
	from = fromList[0]

	// Check that the from and to addresses are formatted correctly.
	addr, err := mail.ParseAddress(from)
	if err != nil {
		return nil, fmt.Errorf("Invalid From address %v.", from)
	}
	switch addr.Address {
	case BmagentAddress:
		from = strings.Split(BmagentAddress, "@")[0]
	default:
		from, err = emailToBM(addr.Address)
		if err != nil {
			return nil, errors.New(
				fmt.Sprintf("From address must be of the form %s.", emailRegexString))
		}
	}

	addr, err = mail.ParseAddress(to)
	if err != nil {
		return nil, fmt.Errorf("Invalid To address %v.", to)
	}
	switch addr.Address {
	case BroadcastAddress:
		to = ""
	case BmagentAddress:
		to = strings.Split(BmagentAddress, "@")[0]
	default:
		to, err = emailToBM(to)
		if err != nil {
			return nil, errors.New(fmt.Sprintf(
			"To address must be of the form %s, %s, or %s.", BroadcastAddress, BmagentAddress))
		}
	}

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

	return &Bitmessage{
		From:       from,
		To:         to,
		Expiration: expiration,
		Ack:        nil,
		Message: &format.Encoding2{
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
func NewBitmessageDraftFromSMTP(smtp *data.Content) (*Bitmessage, error) {
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

	return &Bitmessage{
		From:       from,
		To:         to,
		Expiration: expiration,
		Ack:        nil,
		Message: &format.Encoding2{
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
