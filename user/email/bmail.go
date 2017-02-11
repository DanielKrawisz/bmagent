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

	"github.com/DanielKrawisz/bmutil"
	"github.com/DanielKrawisz/bmutil/cipher"
	"github.com/DanielKrawisz/bmutil/format"
	"github.com/DanielKrawisz/bmutil/identity"
	"github.com/jordwest/imap-server/types"
	"github.com/mailhog/data"
)

const (
	// DateFormat is the format for encoding dates.
	DateFormat = "Mon Jan 2 15:04:05 -0700 MST 2006"

	// Pattern used for matching Bitmessage addresses.
	bmAddrPattern = "BM-[123456789abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ]+"

	commandPattern = "[a-z]+"

	// Broadcast is the email address used to send broadcasts.
	Broadcast = "broadcast@bm.addr"
)

var (
	// ErrInvalidEmail is the error returned by ToBM when the e-mail
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

	// BitmessageRegex is the regex of a Bitmessage address.
	BitmessageRegex = regexp.MustCompile(fmt.Sprintf("^%s$", bmAddrPattern))

	emailRegexString = fmt.Sprintf("^%s@bm\\.addr$", bmAddrPattern)

	// emailRegex is used for extracting Bitmessage address from an e-mail
	// address.
	emailRegex = regexp.MustCompile(emailRegexString)

	commandRegexString = fmt.Sprintf("^%s@bm\\.agent$", commandPattern)

	// CommandRegex is used for detecting an email intended as a
	// command to bmagent.
	CommandRegex = regexp.MustCompile(commandRegexString)
)

// BmToEmail converts a Bitmessage address to an e-mail address.
func BmToEmail(bmAddr string) string {
	if bmAddr == "" {
		return Broadcast
	}
	return fmt.Sprintf("%s@bm.addr", bmAddr)
}

// ToBm extracts a Bitmessage address from an e-mail address.
func ToBm(emailAddr string) (string, error) {
	addr, err := mail.ParseAddress(emailAddr)
	if err != nil {
		return "", err
	}

	if emailAddr == Broadcast {
		return "", nil
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

// ValidateEmail validates an email TO header entry.
func ValidateEmail(to string) bool {
	addr, err := mail.ParseAddress(to)
	if err != nil {
		return false
	}

	if emailRegex.Match([]byte(addr.Address)) {
		return true
	}

	if CommandRegex.Match([]byte(addr.Address)) {
		return true
	}

	return false
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
	Ack        []byte
	Content    format.Encoding
	ImapData   *ImapData
	State      *MessageState
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
		From:       BmToEmail(fromAddress),
		To:         BmToEmail(toAddress),
		Expiration: header.Expiration(),
		OfChannel:  ofChan,
		Content:    data.Content,
		Ack:        msg.Ack(),
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
		From:       Broadcast,
		To:         BmToEmail(fromAddress),
		Expiration: header.Expiration(),
		Content:    data.Content,
	}, nil
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

	headers["To"] = []string{m.To}

	headers["Date"] = []string{m.ImapData.TimeReceived.Format(DateFormat)}
	headers["Expires"] = []string{m.Expiration.Format(DateFormat)}
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

	if !(ValidateEmail(fromList[0]) || ValidateEmail(toList[0])) {
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
		exp, err := time.Parse(DateFormat, expireStr[0])
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

	body, err := GetSMTPBody(smtp)
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
		State: &MessageState{},
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
		exp, err := time.Parse(DateFormat, expireStr[0])
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

	body, err := GetSMTPBody(smtp)
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
		State: &MessageState{
			// false if broadcast; Code for setting it false if sending to
			// channel/self is in GenerateObject.
			AckExpected: to != "",
		},
	}, nil
}
