// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

import (
	"errors"

	"github.com/jordwest/imap-server/mailstore"
	"github.com/jordwest/imap-server/types"
	"github.com/DanielKrawisz/bmagent/message/format"
	"github.com/DanielKrawisz/bmagent/store"
)

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
	imapLog.Tracef("imap authentication attempt with u=%s, p=%s", username, password)
	
	// TODO Use constant time comparisons.
	if username != s.cfg.Username || password != s.cfg.Password {
		return nil, errors.New("Invalid credentials")
	}

	return s.user, nil
}

// InitializeStore initializes the store by creating the default mailboxes and
// inserting the welcome message.
func InitializeUser(u *store.UserData) error {
	
	// Create Inbox.
	mbox, err := u.NewFolder(InboxFolderName)
	if err != nil {
		return err
	}
	inbox, err := NewMailbox(mbox)
	if err != nil {
		return err
	}

	// Add the introductory message.
	from := BmagentAddress
	to := from
	subject := "Welcome to bmagent!"

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

	_, err = u.NewFolder(OutboxFolderName)
	if err != nil {
		return err
	}
	_, err = u.NewFolder(SentFolderName)
	if err != nil {
		return err
	}
	_, err = u.NewFolder(LimboFolderName)
	if err != nil {
		return err
	}
	_, err = u.NewFolder(TrashFolderName)
	if err != nil {
		return err
	}
	_, err = u.NewFolder(CommandsFolderName)
	if err != nil {
		return err
	}
	_, err = u.NewFolder(DraftsFolderName)
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
