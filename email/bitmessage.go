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
	"github.com/monetas/bmclient/store"
	"github.com/monetas/bmutil"
	"github.com/monetas/bmutil/cipher"
	"github.com/monetas/bmutil/identity"
	"github.com/monetas/bmutil/pow"
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

	defaultMsgExpiration = time.Hour * 60

	defaultBroadcastExpiration = time.Hour * 48
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

// ToEmail converts a Bitmessage into an IMAPEmail.
func (be *Bitmessage) ToEmail() (*IMAPEmail, error) {
	var payload *format.Encoding2
	switch m := be.Message.(type) {
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

func getExpireTime(expiration *time.Time, defaultExpiration time.Duration) time.Time {
	if expiration.IsZero() {
		return time.Now().Add(defaultExpiration)
	}
	return *expiration
}

func generateMsgBroadcast(be *Bitmessage, from *identity.Private) (*wire.MsgObject, uint64, uint64, error) {
	// TODO make a separate function in bmutil that does this.
	var signingKey, encKey wire.PubKey
	var tag wire.ShaHash
	sk := from.SigningKey.PubKey().SerializeUncompressed()[1:]
	ek := from.EncryptionKey.PubKey().SerializeUncompressed()[1:]
	t := from.Address.Tag()
	copy(signingKey[:], sk)
	copy(encKey[:], ek)
	copy(tag[:], t)

	broadcast := &wire.MsgBroadcast{
		ObjectType:  wire.ObjectTypeBroadcast,
		Version:     5, // TODO should we give users the option of making version 4 broadcasts?
		ExpiresTime: getExpireTime(&be.Expiration, defaultBroadcastExpiration),

		StreamNumber:       from.Address.Stream, // TODO What is this field? It's not listed in the wiki!
		FromStreamNumber:   from.Address.Stream,
		FromAddressVersion: from.Address.Version,
		NonceTrials:        from.NonceTrialsPerByte,
		ExtraBytes:         from.ExtraBytes,
		SigningKey:         &signingKey,
		EncryptionKey:      &encKey,

		Encoding: be.Message.Encoding(),
		Message:  be.Message.Message(),
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

func generateMsgMsg(be *Bitmessage, from *identity.Private, to *identity.Public) (*wire.MsgObject, uint64, uint64, error) {
	// TODO make a separate function in bmutil that does this.
	var signingKey, encKey wire.PubKey
	var destination wire.RipeHash
	sk := from.SigningKey.PubKey().SerializeUncompressed()[1:]
	ek := to.EncryptionKey.SerializeUncompressed()[1:]
	copy(signingKey[:], sk)
	copy(encKey[:], ek)
	copy(destination[:], to.Address.Ripe[:])

	message := &wire.MsgMsg{
		ObjectType:  wire.ObjectTypeMsg,
		ExpiresTime: getExpireTime(&be.Expiration, defaultMsgExpiration),
		Version:     1,

		StreamNumber:       from.Address.Stream, // TODO What is this field? It's not listed in the wiki!
		FromStreamNumber:   from.Address.Stream,
		FromAddressVersion: from.Address.Version,
		NonceTrials:        from.NonceTrialsPerByte,
		ExtraBytes:         from.ExtraBytes,
		SigningKey:         &signingKey,
		EncryptionKey:      &encKey,
		Destination:        &destination,

		Encoding: be.Message.Encoding(),
		Message:  be.Message.Message(),
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
func (be *Bitmessage) GenerateObject(book keymgr.AddressBook) (object *wire.MsgObject,
	nonceTrials, extraBytes uint64, genErr error) {
	from, err := book.LookupPrivateIdentity(be.From)
	if err != nil {
		smtpLog.Error("GenerateObject: Error returned private identity: ", err)
		return nil, 0, 0, err
	}

	if be.To == "" {
		object, nonceTrials, extraBytes, genErr = generateMsgBroadcast(be, &(from.Private))
	} else {
		to, err := book.LookupPublicIdentity(be.To)
		if err != nil {
			smtpLog.Error("GenerateObject: results of lookup public identity: ", to, ", ", err)
			return nil, 0, 0, err
		}
		// Indicates that a pubkey request was sent.
		if to == nil {
			return nil, 0, 0, nil
		}
		object, nonceTrials, extraBytes, genErr = generateMsgMsg(be, &(from.Private), to)
	}

	if genErr != nil {
		smtpLog.Error("GenerateObject: ", err)
		return nil, 0, 0, genErr
	}
	be.object = object
	return object, nonceTrials, extraBytes, nil
}

// SubmitPow attempts to submit a message for pow.
func (be *Bitmessage) SubmitPow(powQueue *store.PowQueue, addr keymgr.AddressBook) (uint64, error) {
	// Attempt to generate the wire.Object form of the message.
	obj, nonceTrials, extraBytes, err := be.GenerateObject(addr)
	if obj == nil {
		smtpLog.Error("SubmitPow: could not generate message. Pubkey request sent? ", err == nil)
		return 0, err
	}

	// If we were able to generate the object, put it in the pow queue.
	var index uint64
	if obj != nil {
		encoded := wire.EncodeMessage(obj)
		target := pow.CalculateTarget(uint64(len(encoded)),
			uint64(obj.ExpiresTime.Sub(time.Now()).Seconds()), nonceTrials, extraBytes)
		index, err = powQueue.Enqueue(target, encoded[8:])
		if err != nil {
			return 0, err
		}

		if be.state == nil {
			be.state = &MessageState{
				AckExpected: true,
			}
		}
		be.state.PowIndex = index
	}

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
		dateReceived, err := time.Parse(dateFormat, msg.ImapData.TimeReceived)
		if err != nil {
			return nil, err
		}

		l.ImapData = &IMAPData{
			Flags:        types.Flags(msg.ImapData.Flags),
			DateReceived: dateReceived,
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
		From:       from,
		To:         to,
		Expiration: expiration,
		Ack:        nil,
		Message: &format.Encoding2{
			Subject: subject,
			Body:    body,
		},
		state: &MessageState{
			AckExpected: true,
		},
	}, nil
}
