// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

import (
	"net/textproto"
	"time"

	"github.com/jordwest/imap-server/mailstore"
	"github.com/jordwest/imap-server/types"
	"github.com/mailhog/data"
)

// IMAPEmail is a representaton of an email that is compatible with the IMAP
// protocol and is an implementation of mailstore.Message. An email according
// IMAP is associated with a particular mailbox and therefore includes things
// like a uid and sequence number.
type IMAPEmail struct {
	ImapSequenceNumber uint32
	ImapUID            uint64
	ImapFlags          types.Flags
	Date               time.Time
	Mailbox            IMAPMailbox
	Content            *data.Content
}

// Header returns the message's MIME headers as a map in a format compatible
// with imap-server.
func (e *IMAPEmail) Header() textproto.MIMEHeader {
	return textproto.MIMEHeader(e.Content.Headers)
}

// UID return the unique id of the email.
func (e *IMAPEmail) UID() uint32 { return uint32(e.ImapUID) }

// SequenceNumber returns the sequence number of the email.
func (e *IMAPEmail) SequenceNumber() uint32 { return e.ImapSequenceNumber }

// Size returns the RFC822 size of the message.
func (e *IMAPEmail) Size() uint32 {
	return uint32(e.Content.Size)
}

// InternalDate return the date the email was received by the server
// (This is not the date on the envelope of the email).
func (e *IMAPEmail) InternalDate() time.Time { return e.Date }

// Body returns the body of the email.
func (e *IMAPEmail) Body() string {
	body, _ := getSMTPBody(e.Content)
	return body
}

// Keywords returns the list of custom keywords/flags for this message.
func (e *IMAPEmail) Keywords() []string {
	return make([]string, 0)
}

// Flags returns the flags for this message.
func (e *IMAPEmail) Flags() types.Flags {
	return e.ImapFlags
}

// OverwriteFlags overwrites the flags for this message and return the updated
// message.
func (e *IMAPEmail) OverwriteFlags(newFlags types.Flags) mailstore.Message {
	e.ImapFlags = newFlags
	return e
}

// AddFlags writes the flags for this message and return the updated message.
func (e *IMAPEmail) AddFlags(newFlags types.Flags) mailstore.Message {
	e.ImapFlags = e.ImapFlags.SetFlags(newFlags)
	return e
}

// RemoveFlags writes the flags for this message and return the updated message.
func (e *IMAPEmail) RemoveFlags(newFlags types.Flags) mailstore.Message {
	e.ImapFlags = e.ImapFlags.ResetFlags(newFlags)
	return e
}

// SetBody sets the body of the message.
func (e *IMAPEmail) SetBody(body string) mailstore.Message {
	e.Content.Body = body
	return e
}

// SetHeaders sets the headers of a message.
func (e *IMAPEmail) SetHeaders(headers textproto.MIMEHeader) mailstore.Message {
	e.Content.Headers = headers
	return e
}

// Save saves any changes to the message.
func (e *IMAPEmail) Save() (mailstore.Message, error) {
	return e, e.Mailbox.Save(e)
}
