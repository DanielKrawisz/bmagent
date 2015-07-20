// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package message

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/jordwest/imap-server/types"
	"github.com/mailhog/data"
	"github.com/monetas/bmclient/email"
	"github.com/monetas/bmclient/keymgr"
	"github.com/monetas/bmclient/message/format"
	"github.com/monetas/bmclient/message/serialize"
	"github.com/monetas/bmutil"
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
	bmAddrRegex = regexp.MustCompile(fmt.Sprintf("$%s^", bmAddrPattern))
	emailRegex  = regexp.MustCompile(fmt.Sprintf("$(%s)@bm\\.addr^", bmAddrPattern))

	ErrInvalidBitmessageAddress = errors.New("Invalid bitmessage address")
)

func BMToEmail(bmAddr string) string {
	return fmt.Sprintf("%s@bm.addr", bmAddr)
}

func EmailToBM(emailAddr string) (string, error) {
	matches := emailRegex.FindStringSubmatch(emailAddr)
	if len(matches) < 2 {
		return "", ErrInvalidBitmessageAddress
	}

	if _, err := bmutil.DecodeAddress(matches[1]); err != nil {
		return "", ErrInvalidBitmessageAddress
	}
	return matches[1], nil
}

// ImapData provides a Bitmessage with extra information to make it
// compatible with imap.
type ImapData struct {
	UID            uint32
	SequenceNumber uint32
	Flags          types.Flags
	DateReceived   time.Time
	Folder         *Folder
}

// Bitmessage represents a message compatible with a bitmessage format
// (msg or broadcast). If To is nil, then it is a broadcast.
type Bitmessage struct {
	From        *string
	To          *string
	Expiration  *time.Time
	Ack         *string
	Sent        bool
	AckExpected bool
	AckReceived bool
	Payload     format.Encoding
	ImapData    *ImapData
}

// Serialize encodes the message in a protobuf format.
func (m *Bitmessage) Serialize() ([]byte, error) {
	var expr string
	if m.Expiration != nil {
		expr = (*m.Expiration).Format(DateFormat)
	}
	var imapData *serialize.ImapData
	if m.ImapData != nil {
		date := m.ImapData.DateReceived.Format(DateFormat)

		imapData = &serialize.ImapData{
			DateReceived: &date,
			Flags:        (*int32)(&m.ImapData.Flags),
		}
	}
	encode := &serialize.Message{
		From:        m.From,
		To:          m.To,
		Expiration:  &expr,
		Ack:         m.Ack,
		Sent:        &m.Sent,
		AckExpected: &m.AckExpected,
		AckReceived: &m.AckReceived,
		ImapData:    imapData,
		Payload:     m.Payload.ToProtobuf(),
	}

	data, err := proto.Marshal(encode)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// ToEmail converts a Bitmessage into an ImapEmail.
func (be *Bitmessage) ToEmail() (*email.ImapEmail, error) {
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

	headers["From"] = []string{BMToEmail(*be.From)}

	if be.To == nil {
		headers["To"] = []string{"broadcast@bm.addr"}
	} else {
		headers["To"] = []string{BMToEmail(*be.To)}
	}

	if be.Expiration != nil {
		headers["Expires"] = []string{be.Expiration.Format(DateFormat)}
	}

	content := &data.Content{
		Headers: headers,
		Body:    payload.Body,
	}

	email := &email.ImapEmail{
		ImapSequenceNumber: be.ImapData.SequenceNumber,
		ImapUID:            be.ImapData.UID,
		ImapFlags:          be.ImapData.Flags,
		ImapDate:           be.ImapData.DateReceived,
		ImapFolder:         be.ImapData.Folder,
		Content:            content,
	}

	// Calculate the size of the message.
	content.Size = len(fmt.Sprintf("%s\r\n", email.Header())) + len(payload.Body)

	return email, nil
}

// TODO: Must generate the correct tag and send to pow queue.
func generateMsgBroadcast(content []byte, from *identity.Private) (wire.Message, error) {
	return nil, nil
}

// TODO: must generate an ack if none exists and send to pow queue.
func generateMsgMsg(content []byte, from *identity.Private, to *identity.Public) (wire.Message, error) {
	return nil, nil
}

// ToMessage converts the message to wire format.
func ToMessage(msg *Bitmessage, book keymgr.AddressBook) (wire.Message, error) {
	msgContent := msg.Payload.Message()
	from, err := book.LookupPrivateIdentity(msg.From)
	if err != nil {
		return nil, err
	}

	if msg.To == nil {
		return generateMsgBroadcast(msgContent, from)
	}

	to, err := book.LookupPublicIdentity(msg.To)
	if err != nil {
		return nil, err
	}

	return generateMsgMsg(msgContent, from, to)
}

// TODO This function assumes the message has already been decrypted.
// find out if that is automatic.
func MsgRead(msg *wire.MsgMsg, toAddress string) (*Bitmessage, error) {
	payload, err := format.DecodeObjectPayload(msg.Encoding, msg.Message)
	if err != nil {
		return nil, err
	}

	from := bmutil.Address{
		Version: msg.FromAddressVersion,
		Stream:  msg.FromStreamNumber,
		Ripe:    *msg.Destination,
	}
	fromAddress, err := from.Encode()
	if err != nil {
		return nil, err
	}

	ack := string(msg.Ack)

	return &Bitmessage{
		From:       &fromAddress,
		To:         &toAddress,
		Expiration: &msg.ExpiresTime,
		Ack:        &ack,
		Payload:    payload,
	}, nil
}

// TODO This function assumes the message has already been decrypted.
// find out if that is automatic.
func BroadcastRead(msg *wire.MsgBroadcast) (*Bitmessage, error) {
	payload, err := format.DecodeObjectPayload(msg.Encoding, msg.Message)
	if err != nil {
		return nil, err
	}

	// TODO how do we give the from address on a brodcast?
	/*from := bmutil.Address{
		Version: msg.FromAddressVersion,
		Stream:  msg.FromStreamNumber,
		Ripe:    *msg.Destination,
	}
	fromAddress, err := from.Encode()
	if err != nil {
		return nil, err
	}*/

	return &Bitmessage{
		//From:       &fromAddress,
		Expiration: &msg.ExpiresTime,
		Payload:    payload,
	}, nil
}

// DecodeBitmessage takes the protobuf encoding of a bitmessage
// and converts it back to a bitmessage.
func DecodeBitmessage(data []byte) (*Bitmessage, error) {
	msg := &serialize.Message{}
	err := proto.Unmarshal(data, msg)
	if err != nil {
		return nil, err
	}

	var q format.Encoding
	switch *msg.Payload.Format {
	case 1:
		r := &format.Encoding1{}

		if msg.Payload.Body == nil {
			return nil, errors.New("Body required in encoding format 2")
		}
		r.Body = *msg.Payload.Body

		q = r
	case 2:
		r := &format.Encoding2{}

		if msg.Payload.Subject == nil {
			return nil, errors.New("Subject required in encoding format 2")
		}
		r.Subject = *msg.Payload.Subject

		if msg.Payload.Body == nil {
			return nil, errors.New("Body required in encoding format 2")
		}
		r.Body = *msg.Payload.Body

		q = r
	default:
		return nil, errors.New("Unsupported encoding")
	}

	l := &Bitmessage{}

	if msg.Expiration != nil {
		expr, _ := time.Parse(DateFormat, *msg.Expiration)
		l.Expiration = &expr
	}

	l.From = msg.From
	l.To = msg.To
	l.Ack = msg.Ack
	l.Payload = q

	if msg.ImapData != nil {
		dateReceived, err := time.Parse(DateFormat, *msg.ImapData.DateReceived)
		if err != nil {
			return nil, err
		}

		l.ImapData = &ImapData{
			Flags:        types.Flags(*msg.ImapData.Flags),
			DateReceived: dateReceived,
		}
	}

	return l, nil
}

// NewBitmessageFromSMTP takes a SMTP email and turns it into a bitmessage.
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
	if !emailRegex.MatchString(from) {
		return nil, errors.New("From address must be of the form <bm-address>@bm.addr")
	}
	from = bmAddrRegex.FindString(from)
	if _, err := bmutil.DecodeAddress(from); err != nil {
		return nil, errors.New("Invalid bitmessage from address.")
	}

	var toPtr *string
	if emailRegex.MatchString(to) {
		to, err := EmailToBM(to)
		if err != nil {
			return nil, errors.New("Invalid bitmessage to address.")
		}
		toPtr = &to
	} else if to == broadcastAddress {
		toPtr = nil
	} else {
		return nil, errors.New("To address must be of the form <bm-address>@bm.addr or broadcast@bm.addr")
	}

	// Check Reply-To, and if it is set it must be the same as From.
	if replyTo, ok := header["Reply-To"]; ok && len(replyTo) > 0 {
		if len(replyTo) > 1 {
			return nil, errors.New("Invalid headers: Reply-To may have no more than one value.")
		} else if replyTo[0] != from {
			return nil, errors.New("Invalid headers: Reply-To must match From or be unset.")
		}
	}

	// If cc or bcc are set, give an error because these headers cannot
	// be made to work as the user would expect with the Bitmessage protocol
	// as it is currently defined.
	Cc, okCc := header["Cc"]
	Bcc, okBcc := header["Bcc"]
	if (okCc && len(Cc) > 0) || (okBcc && len(Bcc) > 0) {
		return nil, errors.New("Invalid headers: do not use Cc or Bcc with bitmessage")
	}

	// Expires is a rarely-used header that is relevant to Bitmessage.
	// If it is set, use it to generate the expire time of the message.
	// Otherwise, use the default.
	var expiration *time.Time
	if expireStr, ok := header["Expires"]; !ok {
		expiration = nil
	} else {
		exp, err := time.Parse(DateFormat, expireStr[0])
		if err != nil {
			return nil, err
		}
		expiration = &exp
	}

	var subject string
	if subj, ok := header["Subject"]; ok {
		subject = subj[0]
	} else {
		subject = ""
	}

	body, err := email.GetBody(smtp)
	if err != nil {
		return nil, err
	}

	return &Bitmessage{
		From:        &from,
		To:          toPtr,
		Expiration:  expiration,
		Ack:         nil,
		AckExpected: true,
		Payload: &format.Encoding2{
			Subject: subject,
			Body:    body,
		},
	}, nil
}
