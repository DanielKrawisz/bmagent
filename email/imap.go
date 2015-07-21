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
	//Receive(smtp *data.Message, flags types.Flags) (*ImapEmail, error)
	Save(*IMAPEmail) error
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
	from := "bmclient-team"
	to := "you"
	subject := "Welcome to bmclient!"

	err = inbox.AddNew(&Bitmessage{
		From: from,
		To:   to,
		Payload: &format.Encoding2{
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

	return nil
}

// NewBitmessageStore creates a new bitmessage store.
func NewBitmessageStore(user *User, cfg *IMAPConfig) *BitmessageStore {
	return &BitmessageStore{
		user: user,
		cfg:  cfg,
	}
}

// User implements the email.IMAPAccount interface and represents
// a collection of imap folders belonging to a single user.
type User struct {
	boxes map[string]*Mailbox
}

// UserFromStore creates a User object from the store.
func UserFromStore(s *store.Store) (*User, error) {
	u := &User{
		boxes: make(map[string]*Mailbox),
	}

	mboxes := s.Mailboxes()
	for _, mbox := range mboxes {
		mb, err := NewMailbox(mbox)
		if err != nil {
			return nil, err
		}
		u.boxes[mb.Name()] = mb
	}

	return u, nil
}

// NewMailbox adds a new mailbox.
func (u *User) NewMailbox(name string) (*Mailbox, error) {
	return nil, errors.New("Not yet implemented.")
}

// Mailboxes returns all the mailboxes. It is part of the IMAPMailbox interface.
func (u *User) Mailboxes() []mailstore.Mailbox {
	mboxes := make([]mailstore.Mailbox, 0, len(u.boxes))
	for _, mbox := range u.boxes {
		mboxes = append(mboxes, mbox)
	}
	return mboxes
}

// MailboxByName returns a mailbox by its name. It is part of the IMAPMailbox
// interface.
func (u *User) MailboxByName(name string) (mailstore.Mailbox, error) {
	// Enforce the case insensitivity of Inbox.
	if strings.ToLower(name) == strings.ToLower(InboxFolderName) {
		return u.boxes[InboxFolderName], nil
	}

	mbox, ok := u.boxes[name]
	if !ok {
		return nil, errors.New("Not found")
	}
	return mbox, nil
}
