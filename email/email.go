// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

import (
	"errors"
	"net/textproto"
	"time"

	"github.com/jordwest/imap-server/mailstore"
	"github.com/jordwest/imap-server/types"
	"github.com/mailhog/data"
)

// GetBody return the body of the email.
func GetBody(email *data.Content) (string, error) {
	if version, ok := email.Headers["MIME-Version"]; ok {
		if version[0] != "1.0" {
			return "", errors.New("Unrecognized MIME version")
		}

		if contentType, ok := email.Headers["Content-Type"]; !ok {
			return "", errors.New("Unrecognized MIME version")
		} else if contentType[0] != "text/plain" {
			// TODO we should be able to support html bodies.
			return "", errors.New("Unsupported Content-Type; use text/plain")
		} else {
			return email.MIME.Parts[0].Body, nil
		}
	}

	return email.Body, nil
}

// ImapEmail is a representaton of an email that is compatible with the IMAP
// protocol and is an implementation of mailstore.Message. An email according
// IMAP is associated with a particular mailbox and therefore includes things
// like a uid and sequence number.
type ImapEmail struct {
	ImapSequenceNumber uint32
	ImapUID            uint64
	ImapFlags          types.Flags
	Date               time.Time
	Folder             ImapFolder
	Content            *data.Content
}

// Header returns the message's MIME headers as a map in a format compatible
// with imap-server.
func (e *ImapEmail) Header() textproto.MIMEHeader {
	return textproto.MIMEHeader(e.Content.Headers)
}

// UID return the unique id of the email.
func (e *ImapEmail) UID() uint32 { return uint32(e.ImapUID) }

// SequenceNumber returns the sequence number of the email.
func (e *ImapEmail) SequenceNumber() uint32 { return e.ImapSequenceNumber }

// Size returns the RFC822 size of the message.
func (e *ImapEmail) Size() uint32 {
	return uint32(e.Content.Size)
}

// InternalDate return the date the email was received by the server
// (This is not the date on the envelope of the email).
func (e *ImapEmail) InternalDate() time.Time { return e.Date }

// Body returns the body of the email.
func (e *ImapEmail) Body() string {
	body, _ := GetBody(e.Content)
	return body
}

// Keywords returns the list of custom keywords/flags for this message.
func (e *ImapEmail) Keywords() []string {
	return make([]string, 0)
}

// Flags returns the flags for this message.
func (e *ImapEmail) Flags() types.Flags {
	return e.ImapFlags
}

// OverwriteFlags overwrites the flags for this message and return the updated
// message.
func (e *ImapEmail) OverwriteFlags(newFlags types.Flags) mailstore.Message {
	e.ImapFlags = newFlags
	return e
}

// AddFlags writes the flags for this message and return the updated message.
func (e *ImapEmail) AddFlags(newFlags types.Flags) mailstore.Message {
	e.ImapFlags = e.ImapFlags.SetFlags(newFlags)
	return e
}

// RemoveFlags writes the flags for this message and return the updated message.
func (e *ImapEmail) RemoveFlags(newFlags types.Flags) mailstore.Message {
	e.ImapFlags = e.ImapFlags.ResetFlags(newFlags)
	return e
}

// SetBody sets the body of the message.
func (e *ImapEmail) SetBody(body string) mailstore.Message {
	e.Content.Body = body
	return e
}

// SetHeaders sets the headers of a message.
func (e *ImapEmail) SetHeaders(headers textproto.MIMEHeader) mailstore.Message {
	e.Content.Headers = headers
	return e
}

// Save saves any changes to the message.
func (e *ImapEmail) Save() (mailstore.Message, error) {
	return e, e.Folder.Save(e)
}
