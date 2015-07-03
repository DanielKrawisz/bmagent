// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package message

import (
	"errors"
	"fmt"
	"time"

	"github.com/jordwest/imap-server/types"
	"github.com/mailhog/data"
	"github.com/monetas/bmclient/message/email"
	"github.com/monetas/bmclient/message/serialize"
	"github.com/monetas/bmutil/wire"
)

// ErrPublicIdentityRequested means that the message could not be sent yet
// because the public key needs to be requested.
var ErrPublicIdentityRequested = errors.New("Public key has been requested")

// Encoding2 implements the Bitmessage interface and represents a
// MsgMsg or MsgBroadcast with encoding type 2.
type Encoding2 struct {
	from        string
	to          string
	subject     string
	body        string
	ack         *string
	expiration  *time.Time
	nonceTrials uint64
	extraBytes  uint64
	behavior    uint64
}

// Body returns the body of the message.
func (l *Encoding2) Body() string {
	return l.body
}

// Expiration returns the expiration time of the message. May be nil.
func (l *Encoding2) Expiration() time.Time {
	return *l.expiration
}

// Encoding returns the encoding format of the bitmessage.
func (l *Encoding2) Encoding() uint64 {
	return 2
}

// toProtobuf encodes the message in a protobuf format.
func (l *Encoding2) toProtobuf() *serialize.Encoding {
	encoding := uint64(2)
	encode := &serialize.Encoding{
		Encoding:    &encoding,
		From:        &l.from,
		To:          &l.to,
		Subject:     &l.subject,
		Body:        &l.body,
		Ack:         l.ack,
		NonceTrials: &l.nonceTrials,
		ExtraBytes:  &l.extraBytes,
		Behavior:    &l.behavior,
	}

	expiration := l.expiration.Format(dateFormat)
	if l.expiration != nil {
		encode.Expiration = &expiration
	}

	return encode
}

// ToMessage converts the message to a wire form that can be sent over the
// bitmessage network.
//TODO ToMessage converts a message encoded in format 2 into a wire.Message.
//TODO Allow for MsgBroadcast. Right now we assume MsgMsg.
func (l *Encoding2) ToMessage() (wire.Message, error) {
	// Get the identities corresponding to the addresses.
	/*private, err := GetPrivateIdentity(l.from) // dummy function
	if err != nil {
		return nil, err
	}

	public, requested, err := GetPublicIdentity(l.to) // dummy function
	if err != nil {
		return nil, err
	}
	if requested {
		return nil, ErrPublicIdentityRequested
	}

	return wire.NewMsgMsg(0, l.exipiration, 1, public.Stream, nil, public.Version,
		private.Stream, l.behavior, private.SigningKey, public.EncryptionKey,
		l.nonceTrials, l.extraBytes, public.Ripe, 2,
		fmt.Sprintf("Subject:%s\nBody:%s", subject, body), ack, nil), nil*/
	return nil, nil
}

// toEmail converts a bitmessage encoded in format 2 to an email.
func (l *Encoding2) toEmail(uid, sequenceNo uint32, date time.Time,
	flags types.Flags, bmbox BitmessageFolder) *email.ImapEmail {
	headers := make(map[string][]string)

	headers["Subject"] = []string{l.subject}
	if l.expiration != nil {
		headers["Expires"] = []string{l.expiration.Format(dateFormat)}
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
		ImapFolder:         &SMTPFolderWrapper{bmbox},
		Content:            content,
	}

	// Calculate the size of the message.
	content.Size = len(fmt.Sprintf("%s\r\n", email.Header())) + len(l.body)

	return email
}
