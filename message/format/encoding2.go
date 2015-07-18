// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package format

import (
	"errors"
	"fmt"
	"time"

	"github.com/jordwest/imap-server/types"
	"github.com/mailhog/data"
	"github.com/monetas/bmclient/message/email"
	"github.com/monetas/bmclient/message/serialize"
	"github.com/monetas/bmutil"
)

// Encoding2 implements the Bitmessage interface and represents a
// MsgMsg or MsgBroadcast with encoding type 2. It also implements the
type Encoding2 struct {
	from       *string
	to         *string
	subject    string
	body       string
	expiration *time.Time
}

// To returns the to bitmessage address.
// Part of the Bitmessage interface.
func (l *Encoding2) To() *string {
	return l.to
}

// From returns the from bitmessage address.
// Part of the Bitmessage interface.
func (l *Encoding2) From() *string {
	return l.from
}

// Expiration returns the expiration time of the message. May be nil.
func (l *Encoding2) Expiration() *time.Time {
	return l.expiration
}

// Encoding returns the encoding format of the bitmessage.
func (l *Encoding2) Encoding() uint64 {
	return 2
}

// ToProtobuf encodes the message in a protobuf format.
func (l *Encoding2) ToProtobuf() *serialize.Encoding {
	encoding := uint64(2)
	var expr string
	if l.expiration != nil {
		expr = (*l.expiration).Format(DateFormat)
	}
	return &serialize.Encoding{
		Encoding:   &encoding,
		From:       l.from,
		To:         l.to,
		Subject:    &l.subject,
		Body:       &l.body,
		Expiration: &expr,
	}
}

// Message returns the raw form of the object payload.
func (l *Encoding2) Message() []byte {
	return []byte(fmt.Sprintf("Subject:%s\nBody:%s", l.subject, l.body))
}

// ToEmail converts a bitmessage encoded in format 2 to an email.
func (l *Encoding2) ToEmail(uid, sequenceNo uint32, date time.Time,
	flags types.Flags, folder email.ImapFolder) *email.ImapEmail {
	headers := make(map[string][]string)

	headers["Subject"] = []string{l.subject}

	headers["From"] = []string{fmt.Sprintf("%s@bm.addr", &l.from)}

	if l.to == nil {
		headers["To"] = []string{"broadcast@bm.addr"}
	} else {
		headers["To"] = []string{fmt.Sprintf("%s@bm.addr", &l.to)}
	}

	if l.expiration != nil {
		headers["Expires"] = []string{l.expiration.Format(DateFormat)}
	}

	content := &data.Content{
		Headers: headers,
		Body:    l.body,
	}

	email := &email.ImapEmail{
		ImapSequenceNumber: sequenceNo,
		ImapUID:            uid,
		ImapFlags:          flags,
		ImapDate:           date,
		ImapFolder:         folder,
		Content:            content,
	}

	// Calculate the size of the message.
	content.Size = len(fmt.Sprintf("%s\r\n", email.Header())) + len(l.body)

	return email
}

// NewBitmessageFromSMTP takes a SMTP email and turns it into a bitmessage.
func NewBitmessageFromSMTP(smtp *data.Content) (*Encoding2, error) {
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
		to = bmAddrRegex.FindString(to)
		if _, err := bmutil.DecodeAddress(to); err != nil {
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

	return &Encoding2{
		from:       &from,
		to:         toPtr,
		subject:    subject,
		body:       body,
		expiration: expiration,
	}, nil
}
