// Copyright (c) 2015 Monetas.
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
	"github.com/monetas/bmclient/keymgr"
	"github.com/monetas/bmclient/message/format"
	"github.com/monetas/bmclient/message/serialize"
	"github.com/monetas/bmutil"
	"github.com/monetas/bmutil/identity"
	"github.com/monetas/bmutil/wire"
)

const (
	// dateFormat is the format for encoding dates.
	dateFormat = "Mon Jan 2 15:04:05 -0700 MST 2006"

	bmAddrPattern = "BM-[123456789abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ]+"
)

var (
	emailRegex = regexp.MustCompile(fmt.Sprintf("(%s)@bm\\.addr", bmAddrPattern))

	ErrInvalidEmail = errors.New("Invalid Bitmessage address")
)

// BMToEmail converts a Bitmessage address to an e-mail address.
func BMToEmail(bmAddr string) string {
	return fmt.Sprintf("%s@bm.addr", bmAddr)
}

// EmailToBM extracts a Bitmessage address from an e-mail address.
func EmailToBM(emailAddr string) (string, error) {
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
	DateReceived   time.Time
	Mailbox        *Mailbox
}

// Bitmessage represents a message compatible with a bitmessage format
// (msg or broadcast). If To is empty, then it is a broadcast.
type Bitmessage struct {
	From        string
	To          string
	OfChannel   bool // Whether the message was sent to/received from a channel.
	Expiration  time.Time
	Ack         []byte
	Sent        bool
	AckExpected bool
	AckReceived bool
	Payload     format.Encoding
	ImapData    *IMAPData
	// The encoded form of the message as a bitmessage object. Required
	// for messages that are waiting to be sent or have pow done on them.
	object *wire.MsgObject
}

// Serialize encodes the message in a protobuf format.
func (m *Bitmessage) Serialize() ([]byte, error) {
	expr := m.Expiration.Format(dateFormat)

	var imapData *serialize.ImapData
	if m.ImapData != nil {
		date := m.ImapData.DateReceived.Format(dateFormat)

		imapData = &serialize.ImapData{
			TimeReceived: date,
			Flags:        int32(m.ImapData.Flags),
		}
	}

	var object []byte
	if m.object == nil {
		object = nil
	} else {
		object = wire.EncodeMessage(m.object)
	}

	encode := &serialize.Message{
		From:        m.From,
		To:          m.To,
		OfChannel:   m.OfChannel,
		Expiration:  expr,
		Ack:         m.Ack,
		Sent:        m.Sent,
		AckExpected: m.AckExpected,
		AckReceived: m.AckReceived,
		ImapData:    imapData,
		Payload:     m.Payload.ToProtobuf(),
		Object:      object,
	}

	data, err := proto.Marshal(encode)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// ToEmail converts a Bitmessage into an IMAPEmail.
func (be *Bitmessage) ToEmail() (*IMAPEmail, error) {
	var payload *format.Encoding2
	switch m := be.Payload.(type) {
	// Only encoding 2 is considered to be compatible with email.
	case *format.Encoding2:
		payload = m
	default:
		return nil, errors.New("Wrong format")
	}

	if be.ImapData == nil {
		return nil, errors.New("No IMAP data")
	}

	headers := make(map[string][]string)

	headers["Subject"] = []string{payload.Subject}

	headers["From"] = []string{BMToEmail(be.From)}

	if be.To == "" {
		headers["To"] = []string{BroadcastAddress}
	} else {
		headers["To"] = []string{BMToEmail(be.To)}
	}

	headers["Date"] = []string{be.ImapData.DateReceived.Format(dateFormat)}
	headers["Expires"] = []string{be.Expiration.Format(dateFormat)}
	if be.OfChannel {
		headers["Reply-To"] = []string{BMToEmail(be.To)}
	}
	headers["Content-Type"] = []string{`text/plain; charset="UTF-8"`}
	headers["Content-Transfer-Encoding"] = []string{"8bit"}

	content := &data.Content{
		Headers: headers,
		Body:    payload.Body,
	}

	email := &IMAPEmail{
		ImapSequenceNumber: be.ImapData.SequenceNumber,
		ImapUID:            be.ImapData.UID,
		ImapFlags:          be.ImapData.Flags,
		Date:               be.ImapData.DateReceived,
		Mailbox:            be.ImapData.Mailbox,
		Content:            content,
	}

	// Calculate the size of the message.
	content.Size = len(fmt.Sprintf("%s\r\n", email.Header())) + len(payload.Body)

	return email, nil
}

// TODO: If this is a broadcast, must generate the correct tag.
func generateMsgBroadcast(content []byte, from *identity.Private) (*wire.MsgObject, error) {
	return nil, nil
}

// TODO: must generate an ack if none exists and send to pow queue.
func generateMsgMsg(content []byte, from *identity.Private, to *identity.Public) (*wire.MsgObject, error) {
	return nil, nil
}

// GenerateObject generates the wire.MsgObject form of the message.
func (be *Bitmessage) GenerateObject(msg *Bitmessage, book keymgr.AddressBook) (object *wire.MsgObject, genErr error) {
	msgContent := msg.Payload.Message()
	from, err := book.LookupPrivateIdentity(msg.From)
	if err != nil {
		return nil, err
	}

	if msg.To == "" {
		object, genErr = generateMsgBroadcast(msgContent, from)
	} else {
		to, err := book.LookupPublicIdentity(msg.To)
		if err != nil {
			return nil, err
		}
		object, genErr = generateMsgMsg(msgContent, from, to)
	}

	if genErr != nil {
		return nil, err
	}
	be.object = object
	return object, nil
}

// MsgRead creates a Bitmessage object from an unencrypted wire.MsgMsg.
func MsgRead(msg *wire.MsgMsg, toAddress string, ofChan bool) (*Bitmessage, error) {
	payload, err := format.DecodeObjectPayload(msg.Encoding, msg.Message)
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
		Payload:    payload,
		OfChannel:  ofChan,
	}, nil
}

// BroadcastRead creates a Bitmessage object from an unencrypted
// wire.MsgBroadcast.
func BroadcastRead(msg *wire.MsgBroadcast) (*Bitmessage, error) {
	payload, err := format.DecodeObjectPayload(msg.Encoding, msg.Message)
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
		Payload:    payload,
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
	switch msg.Payload.Format {
	case 1:
		r := &format.Encoding1{}

		if msg.Payload.Body == nil {
			return nil, errors.New("Body required in encoding format 1")
		}
		r.Body = string(msg.Payload.Body)

		q = r
	case 2:
		r := &format.Encoding2{}

		if msg.Payload.Subject == nil {
			return nil, errors.New("Subject required in encoding format 2")
		}
		r.Subject = string(msg.Payload.Subject)

		if msg.Payload.Body == nil {
			return nil, errors.New("Body required in encoding format 2")
		}
		r.Body = string(msg.Payload.Body)

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
	l.Payload = q
	if msg.Object != nil {
		l.object, err = wire.DecodeMsgObject(msg.Object)
		if err != nil {
			return nil, err
		}
	}

	if msg.ImapData != nil {
		dateReceived, err := time.Parse(dateFormat, msg.ImapData.TimeReceived)
		if err != nil {
			return nil, err
		}

		l.ImapData = &IMAPData{
			Flags:        types.Flags(msg.ImapData.Flags),
			DateReceived: dateReceived,
		}
	}

	return l, nil
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
	case BmclientAddress:
		from = strings.Split(addr.Address, "@")[0]
	default:
		from, err = EmailToBM(addr.Address)
		if err != nil {
			return nil, errors.New("From address must be of the form <bm-address>@bm.addr")
		}
	}

	addr, err = mail.ParseAddress(to)
	if err != nil {
		return nil, fmt.Errorf("Invalid To address %v.", to)
	}
	switch addr.Address {
	case BroadcastAddress:
		fallthrough
	case BmclientAddress:
		to = strings.Split(addr.Address, "@")[0]
	default:
		to, err = EmailToBM(to)
		if err != nil {
			return nil, errors.New("To address must be of the form <bm-address>@bm.addr or broadcast@bm.addr")
		}
	}

	// TODO FIXME cannot be same for channels
	// Check Reply-To, and if it is set it must be the same as From.
	/*if replyTo, ok := header["Reply-To"]; ok && len(replyTo) > 0 {
		if len(replyTo) > 1 {
			return nil, errors.New("Invalid headers: Reply-To may have no more than one value.")
		} else if replyTo[0] != from {
			return nil, errors.New("Invalid headers: Reply-To must match From or be unset.")
		}
	}*/

	// If cc or bcc are set, give an error because these headers cannot
	// be made to work as the user would expect with the Bitmessage protocol
	// as it is currently defined.
	Cc, okCc := header["Cc"]
	Bcc, okBcc := header["Bcc"]
	if (okCc && len(Cc) > 0) || (okBcc && len(Bcc) > 0) {
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
		From:        from,
		To:          to,
		Expiration:  expiration,
		Ack:         nil,
		AckExpected: true,
		Payload: &format.Encoding2{
			Subject: subject,
			Body:    body,
		},
	}, nil
}
