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
	"github.com/monetas/bmclient/message/email"
	"github.com/monetas/bmclient/message/serialize"
	"github.com/monetas/bmutil/pow"
	"github.com/monetas/bmutil/wire"
)

// TODO use bmutil.DecodeAddress
const bmAddrPattern = "BM-[123456789abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ]+"

var bmAddrRegex, _ = regexp.Compile(bmAddrPattern)
var emailRegex, _ = regexp.Compile(fmt.Sprintf("%s@bm\\.addr", bmAddrRegex))

// The format for encoding dates.
const dateFormat = "Mon Jan 2 15:04:05 -0700 MST 2006"

// Bitmessage represents any bitmessage format that is
// designed to work like email.
type Bitmessage interface {
	Body() string

	Encoding() uint64

	Expiration() time.Time

	// Convert to a form that can be transmitted over the network.
	ToMessage() (wire.Message, error)

	toProtobuf() *serialize.Encoding

	// Convert to an email format.
	toEmail(uid, sequenceNo uint32, date time.Time, flags types.Flags, bmbox BitmessageFolder) *email.ImapEmail
}

// newBitmessageFromSMTP takes a SMTP email and turns it into a bitmessage.
func newBitmessageFromSMTP(smtp *data.Content) (Bitmessage, error) {
	header := smtp.Headers

	// TODO any other headers that should be addressed?
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
	if !emailRegex.MatchString(to) {
		return nil, errors.New("To address must be of the form <bm-address>@bm.addr")
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
	var expiration time.Time
	if expireStr, ok := header["Expires"]; !ok {
		// TODO make this a constant somewhere. Check if this is the same
		// default value as PyBitmessage
		expiration = time.Now().Add(4 * 24 * time.Hour)
	} else {
		var err error
		expiration, err = time.Parse(dateFormat, expireStr[0])
		if err != nil {
			return nil, err
		}
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
		from:        bmAddrRegex.FindString(from),
		to:          bmAddrRegex.FindString(to),
		subject:     subject,
		body:        body,
		expiration:  &expiration,
		nonceTrials: pow.DefaultNonceTrialsPerByte,
		extraBytes:  pow.DefaultExtraBytes,
	}, nil
}

// newBitmessageFromProtobuf takes the protobuf encoding of a bitmessage
// and converts it back to a bitmessage.
func newBitmessageFromProtobuf(msg *serialize.Encoding) (Bitmessage, error) {
	if *msg.Encoding != 2 {
		return nil, errors.New("Unsupported encoding")
	}

	r := &Encoding2{}

	if msg.Subject == nil {
		return nil, errors.New("Subject required in encoding format 2")
	}
	r.subject = *msg.Subject

	if msg.Body == nil {
		return nil, errors.New("Body required in encoding format 2")
	}
	r.body = *msg.Body

	if msg.Expiration == nil {
		r.expiration = nil
	} else {
		*r.expiration, _ = time.Parse(dateFormat, *msg.Expiration)
	}

	r.ack = msg.Ack

	r.from = *msg.From
	r.to = *msg.To
	r.nonceTrials = *msg.NonceTrials
	r.extraBytes = *msg.ExtraBytes
	r.behavior = *msg.Behavior

	return r, nil
}

// BitmessageEntry represents a bitmessage with related information
// attached as to its position and state in a folder.
type BitmessageEntry struct {
	UID            uint32
	SequenceNumber uint32
	Sent           bool
	AckExpected    bool
	AckReceived    bool
	DateReceived   time.Time
	Flags          types.Flags
	Folder         BitmessageFolder
	Message        Bitmessage
}

// ToEmail converts a BitmessageEntry into an ImapEmail.
func (be *BitmessageEntry) ToEmail() *email.ImapEmail {
	return be.Message.toEmail(be.UID, be.SequenceNumber, be.DateReceived, be.Flags, be.Folder)
}

// Encode transforms a BitmessageEntry into a byte array suitable for inserting into the database.
func (be *BitmessageEntry) Encode() (uid uint32, sequenceNumber uint32, encoding uint64, msg []byte, err error) {
	date := be.DateReceived.Format(dateFormat)

	encode := &serialize.Entry{
		Sent:         &be.Sent,
		AckReceived:  &be.AckReceived,
		AckExpected:  &be.AckExpected,
		DateReceived: &date,
		Flags:        (*int32)(&be.Flags),
		Message:      be.Message.toProtobuf(),
	}

	msg, err = proto.Marshal(encode)
	if err != nil {
		return 0, 0, 0, nil, err
	}
	return be.UID, be.SequenceNumber, 2, msg, nil
}

// DecodeBitmessageEntry reconstructs an entry from a byte slice.
func DecodeBitmessageEntry(uid, sequenceNumber uint32, msg []byte) (*BitmessageEntry, error) {
	entry := &serialize.Entry{}
	err := proto.Unmarshal(msg, entry)
	if err != nil {
		return nil, err
	}

	dateReceived, err := time.Parse(dateFormat, *entry.DateReceived)
	if err != nil {
		return nil, err
	}

	message, err := newBitmessageFromProtobuf(entry.Message)
	if err != nil {
		return nil, err
	}

	be := &BitmessageEntry{
		AckExpected:  *entry.AckExpected,
		AckReceived:  *entry.AckReceived,
		Sent:         *entry.Sent,
		Flags:        types.Flags(*entry.Flags),
		DateReceived: dateReceived,
		Message:      message,
	}
	return be, nil
}
