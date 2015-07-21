// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package user

import (
	"errors"
	"fmt"

	"github.com/jordwest/imap-server/mailstore"
	"github.com/jordwest/imap-server/types"
	"github.com/mailhog/data"
	"github.com/monetas/bmclient/email"
	"github.com/monetas/bmclient/message"
	"github.com/monetas/bmclient/message/format"
	"github.com/monetas/bmclient/store"
)

const welcomeMsg = `
Welcome to bmd and to the anonymous, encrypted world of Bitmessage! You are using version 0.1 alpha. 

If you wish to send a bitmessage, just write an email with an address of the form
<bitmessage address>@bm.addr

Have a bug to report? Care to help out? Please see our github repo:
https://github.com/monetas/bmclient/`

var (
	// defaultInboxFolderName is the default name for the inbox folder.
	// the IMAP protocol requires that a folder named INBOX, all caps, exists.
	// So this can't be changed.
	defaultInboxFolderName = "INBOX"

	// defaultOutFolderName is the default name for the out folder.
	// Messages in the out folder are waiting for pow to be completed or public
	// key of the recipient so that they can be sent.
	defaultOutFolderName = "Outbox"

	// defaultLimboFolderName is the default name for folder containing messages
	// that are out in the network, but have not been received yet (no ack).
	defaultLimboFolderName = "Limbo"

	// defaultTrashFolderName is the default name for the trash folder.
	defaultSentFolderName = "Sent"

	// defaultTrashFolderName is the default name for the trash folder.
	defaultTrashFolderName = "Trash"

	// defaultDraftsFolderName is the default name for the drafts folder.
	defaultDraftsFolderName = "Drafts"
)

// BitmessageStore implements mailstore.Authenticate.
type BitmessageStore struct {
	users    map[string]string
	accounts map[string]*ClientAccount
	dbpath   string
}

// Authenticate is part of the mailstore.Authenticate interface. It takes
// a username and password and returns a mailstore.User if the credentials
// are valid.
func (bmstore *BitmessageStore) Authenticate(username string, password string) (mailstore.User, error) {
	if pw, ok := bmstore.users[username]; !ok {
		return nil, errors.New("Invalid username")
	} else if pw != password {
		return nil, errors.New("Invalid password")
	}

	account, ok := bmstore.accounts[username]
	if !ok {
		return nil, errors.New("User account not yet set up")
	}
	return account, nil
}

// AddAccount adds an account to the bitmessage store.
// TODO check if the password is good enough.
func (bmstore *BitmessageStore) AddAccount(username, password string) (*ClientAccount, error) {
	_, ok := bmstore.users[username]
	if ok {
		return nil, errors.New("A user already exists by that name")
	}
	bmstore.users[username] = password

	// TODO use a real mailbox instead.
	var account *ClientAccount
	if bmstore.dbpath == "" {
		account = NewClientAccount(nil, nil)
	} else {
		dataStore, err := store.Open(bmstore.dbpath+"/.db/"+username+".bolt", []byte(password))
		if err != nil {
			return nil, err
		}
		account = NewClientAccount(nil,
			func(folder string) (message.Mailbox, error) {
				return store.NewMailbox(dataStore, folder, true)
			})
	}

	bmstore.accounts[username] = account

	inbox, err := bmstore.NewFolder(username, "INBOX")
	if err != nil {
		return nil, err
	}

	// TODO these are just addresses Daniel made for his own use.
	from := "BM-NBddNS6ZagzjNbMMkVBpecuSAPU1EgyQ@bm.addr"
	to := "BM-NBPVwY5A26MtyfbHyh4UfA4Hn76DamAP@bm.addr"

	_, err = inbox.AddNew(&message.Bitmessage{
		From: from,
		To:   to,
		Payload: &format.Encoding2{
			Subject: "Welcome to bmd!",
			Body:    welcomeMsg,
		},
	}, types.FlagRecent)
	if err != nil {
		fmt.Println("Err making msg:", err)
	}

	account.AddBitmessageFolder(defaultOutFolderName)
	account.AddBitmessageFolder(defaultSentFolderName)
	account.AddBitmessageFolder(defaultLimboFolderName)
	account.AddBitmessageFolder(defaultTrashFolderName)
	account.AddBitmessageFolder(defaultDraftsFolderName)
	return account, nil
}

// NewFolder adds a bitmessage folder to a user's account.
func (bmstore *BitmessageStore) NewFolder(username, boxname string) (*message.Folder, error) {
	_, ok := bmstore.users[username]
	if !ok {
		return nil, errors.New("No user exists by that name.")
	}
	account := bmstore.accounts[username]
	return account.AddBitmessageFolder(boxname)
}

// NewBitmessageStore creates a new bitmessage store.
// dbpath can be set to "" to use memory-only.
func NewBitmessageStore(dbpath string) *BitmessageStore {
	return &BitmessageStore{
		users:    make(map[string]string),
		accounts: make(map[string]*ClientAccount),
		dbpath:   dbpath,
	}
}

// defaultPolicy defines default policy for messages received by SMTP to be
// that all are placed in the default inbox.
func defaultPolicy(msg *message.Bitmessage) *string {
	return &defaultOutFolderName
}

// SendPolicy is a function used to choose into which folder a given bitmessage
// will be inserted when delivered to an account.
type SendPolicy func(msg *message.Bitmessage) *string

// createFolder is a function that tells a bitmessage account how to create
// a new folder.
type createMailbox func(name string) (message.Mailbox, error)

// ClientAccount implements the email.ImapAccount interface and represents
// a collection of imap folders belonging to a single user.
type ClientAccount struct {
	boxes map[string]*message.Folder
	// policy is a function that gives the name of the folder in which
	// an email should be delivered.
	policy SendPolicy
	// A function that tells the account how to create a new folder.
	create createMailbox

	//A queue of items to have pow done on them.
	powQueue store.PowQueue
}

// AddBitmessageFolder adds a new folder to the client account.
func (bf *ClientAccount) AddBitmessageFolder(name string) (*message.Folder, error) {
	_, ok := bf.boxes[name]
	if ok {
		return nil, fmt.Errorf("There is already a folder %s", name)
	}
	if bf.create == nil {
		return nil, errors.New("Cannot create new folder")
	}
	box, err := bf.create(name)
	if err != nil {
		return nil, err
	}
	bmb := message.NewFolder(box)
	if bmb == nil {
		return nil, fmt.Errorf("Failed to create folder %s", name)
	}
	bf.boxes[name] = bmb
	return bmb, nil
}

// Mailboxes is part of the *message.Folder interface. It returns the list
// of mailboxes in the account.
func (bf *ClientAccount) Mailboxes() []mailstore.Mailbox {
	bm := make([]mailstore.Mailbox, len(bf.boxes))

	i := 0
	for _, box := range bf.boxes {
		bm[i] = box
		i++
	}
	return bm
}

// BitmessageFolderByName is part of the *message.Folder interface. It gets
// a folder by name.
func (bf *ClientAccount) BitmessageFolderByName(name string) (*message.Folder, error) {
	box, ok := bf.boxes[name]
	if !ok {
		return nil, errors.New(fmt.Sprint("No mailbox found named ", name))
	}
	return box, nil
}

// MailboxByName returns a mailbox by its name.
// It is part of the *message.Folder interface.
func (bf *ClientAccount) MailboxByName(name string) (mailstore.Mailbox, error) {
	return bf.BitmessageFolderByName(name)
}

// SendMail takes an smtp message and detects whether it is valid for a given
// user and choose which folder to put it in.
// It is part of the *message.Folder interface.
// TODO verify that the format of the email is ok for this user. Does the
// user control the from: address, for example?
func (bf *ClientAccount) SendMail(smtp *data.Message, flags types.Flags) (*email.ImapEmail, error) {
	bitmessage, err := message.NewBitmessageFromSMTP(smtp.Content)
	if err != nil {
		return nil, err
	}

	name := bf.policy(bitmessage)

	box, ok := bf.boxes[*name]
	if !ok {
		box, err = bf.AddBitmessageFolder(*name)
		if err != nil {
			return nil, fmt.Errorf("Could not create new folder: %s", err)
		}
	}

	entry, err := box.AddNew(bitmessage, flags)
	if err != nil {
		return nil, err
	}

	err = box.Send(entry.ImapData.UID)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Failed to send message: %s", err))
	}

	email, err := entry.ToEmail()
	if err != nil {
		return nil, err
	}

	return email, nil
}

// NewClientAccount returns a new client account.
func NewClientAccount(sendPolicy SendPolicy, create createMailbox) *ClientAccount {
	account := &ClientAccount{
		boxes: make(map[string]*message.Folder),
	}

	if sendPolicy == nil {
		account.policy = defaultPolicy
	}

	if create == nil {
		account.create = func(folder string) (message.Mailbox, error) {
			return message.NewMembox(folder), nil
		}
	} else {
		account.create = create
	}

	return account
}
