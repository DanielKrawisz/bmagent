// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

import (
	"github.com/jordwest/imap-server/mailstore"
	"github.com/jordwest/imap-server/types"
	"github.com/mailhog/data"
)

// ImapFolder represents an imap email folder. mailstore.Mailbox only defines
// the functionality required to interact with an imap client, so extra
// functions can be defined here if necessary.
type ImapFolder interface {
	mailstore.Mailbox
	Receive(smtp *data.Message, flags types.Flags) (*ImapEmail, error)
	Save(*ImapEmail) error
}

// ImapAccount represents an imap email user account, which may
// contain multiple folders.
type ImapAccount interface {
	mailstore.User
	Deliver(*data.Message, types.Flags) (*ImapEmail, error)
}
