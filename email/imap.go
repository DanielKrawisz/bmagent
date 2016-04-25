// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

import (
	"fmt"
	"errors"

	"github.com/jordwest/imap-server/mailstore"
	"github.com/jordwest/imap-server/types"
	"github.com/DanielKrawisz/bmagent/message/format"
	"github.com/DanielKrawisz/bmagent/store"
	"github.com/DanielKrawisz/bmagent/keymgr"
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
func InitializeUser(u *store.UserData, keys *keymgr.Manager) error {
	
	// Create Inbox.
	mbox, err := u.NewFolder(InboxFolderName)
	if err != nil {
		return err
	}
	inbox, err := NewMailbox(mbox, keys.Tags())
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
	
	// TODO 
	// Is the key manager empty? 
	// If not, generate a key. 
	if keys.Size() == 0 {
		keys.NewHDIdentity(1)
	}
	
	// Get all keys from key manager. 
	addresses := keys.Addresses()
	tags := keys.Tags()
	
	// For each key, create a mailbox. 
	var toAddr string
	keyList := ""
	
	for addr, _ := range addresses {
		toAddr = addr
		var tag string
		if t, ok := tags[addr]; ok && t != nil {
			tag = *t
		} 
		keyList = fmt.Sprint(keyList, fmt.Sprintf("\t%s@bm.addr %s\n", addr, tag))
	}
	
	welcome := fmt.Sprintf(welcomeMsg, keyList)

	// Add the introductory message.
	from := "welcome@bm.agent"
	subject := "Welcome to bmagent!"
	
	err = inbox.AddNew(&Bitmessage{
		From: from,
		To:   fmt.Sprintf("%s@bm.addr", toAddr),
		Message: &format.Encoding2{
			Subject: subject,
			Body:    welcome,
		},
	}, types.FlagRecent)
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
