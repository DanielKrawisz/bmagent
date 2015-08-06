// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

import (
	"errors"
	"strings"

	"github.com/jordwest/imap-server/mailstore"
	"github.com/jordwest/imap-server/types"
	"github.com/monetas/bmclient/message/format"
	"github.com/monetas/bmclient/store"
)

// IMAPMailbox represents an IMAP e-mail folder. mailstore.Mailbox only defines
// the functionality required to interact with an IMAP client, so extra
// functions can be defined here if necessary.
type IMAPMailbox interface {
	mailstore.Mailbox

	Save(*IMAPEmail) error

	// Refresh updates cached statistics like number of messages in inbox,
	// next UID, last UID, number of recent/unread messages etc. It is meant to
	// be called after the mailbox has been modified by an agent other than the
	// IMAP server. This could be the SMTP server, or new message from bmd.
	Refresh() error
}

// IMAPUser represents an imap e-mail user account, which contains multiple
// folders.
type IMAPUser interface {
	mailstore.User
	//Deliver(*data.Message, types.Flags) (*ImapEmail, error)
}

// IMAPConfig contains configuration options for the IMAP server.
type IMAPConfig struct {
	Username   string
	Password   string
	RequireTLS bool
}

// BitmessageStore implements mailstore.Mailstore.
type BitmessageStore struct {
	cfg  *IMAPConfig
	user *User
}

// Authenticate is part of the mailstore.Mailstore interface. It takes
// a username and password and returns a mailstore.User if the credentials
// are valid.
func (s *BitmessageStore) Authenticate(username string, password string) (mailstore.User, error) {
	// TODO Use constant time comparisons.
	if username != s.cfg.Username || password != s.cfg.Password {
		return nil, errors.New("Invalid credentials")
	}

	return s.user, nil
}

// InitializeStore initializes the store by creating the default mailboxes and
// inserting the welcome message.
func InitializeStore(s *store.Store) error {
	// Create Inbox.
	mbox, err := s.NewMailbox(InboxFolderName)
	if err != nil {
		return err
	}
	inbox, err := NewMailbox(mbox)
	if err != nil {
		return err
	}

	// Add the introductory message.
	from := strings.Split(BmclientAddress, "@")[0]
	to := from
	subject := "Welcome to bmclient!"

	err = inbox.AddNew(&Bitmessage{
		From: from,
		To:   to,
		Message: &format.Encoding2{
			Subject: subject,
			Body:    welcomeMsg,
		},
	}, types.FlagRecent)
	if err != nil {
		return err
	}

	_, err = s.NewMailbox(OutboxFolderName)
	if err != nil {
		return err
	}
	_, err = s.NewMailbox(SentFolderName)
	if err != nil {
		return err
	}
	_, err = s.NewMailbox(LimboFolderName)
	if err != nil {
		return err
	}
	_, err = s.NewMailbox(TrashFolderName)
	if err != nil {
		return err
	}
	_, err = s.NewMailbox(CommandsFolderName)
	if err != nil {
		return err
	}
	_, err = s.NewMailbox(DraftsFolderName)
	if err != nil {
		return err
	}

	return nil
}

// NewBitmessageStore creates a new bitmessage store.
func NewBitmessageStore(user *User, cfg *IMAPConfig) *BitmessageStore {
	return &BitmessageStore{
		user: user,
		cfg:  cfg,
	}
}
