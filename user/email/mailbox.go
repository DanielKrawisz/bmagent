// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

import (
	"github.com/jordwest/imap-server/mailstore"
	"github.com/jordwest/imap-server/types"
)

// Mailbox represent a mailbox compatible with both IMAP and Bitmessage.
type Mailbox interface {
	mailstore.Mailbox

	// Save saves an IMAP email in the Mailbox.
	Save(email *IMAPEmail) error

	// AddNew adds a new Bitmessage to the Mailbox.
	AddNew(bmsg *Bmail, flags types.Flags) error

	// DeleteBitmessageByUID deletes a bitmessage by uid.
	DeleteBitmessageByUID(id uint64) error
}
