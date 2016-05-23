// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

const welcomeMsg = `
Welcome to bmagent and to the anonymous, encrypted world of Bitmessage! You are
using version 0.1 alpha.

You have the following addresses available: 

%s

To send a bitmessage, write an email with an address of the
form <destination bitmessage address>@bm.addr. For sending out a broadcast,
shoot an e-mail to broadcast@bm.addr. Don't forget to add valid From addresses
in your e-mail client! From addresses are considered valid if bmagent holds
private keys for them. You can also send commands to bmagent via the address
<command>@bm.agent.

Have a bug to report? Care to help out? Please see our github repo:
https://github.com/DanielKrawisz/bmagent/`

const newAddressesMsg = `
You have generated the following new addresses:

%s

You can now receive and send messages with these addresses.`

const commandWelcomeMsg = `
(put a list of commands here.)`

const (
	// InboxFolderName is the default name for the inbox folder.
	// The IMAP protocol requires that a folder named Inbox, case insensitive,
	// exist. So this can't be changed.
	InboxFolderName = "Inbox"

	// OutboxFolderName is the default name for the out folder.
	// Messages in the out folder are waiting for pow to be completed or public
	// key of the recipient so that they can be sent.
	OutboxFolderName = "Outbox"

	// LimboFolderName is the default name for folder containing messages
	// that are out in the network, but have not been received yet (no ack).
	LimboFolderName = "Limbo"

	// SentFolderName is the default name for the sent folder.
	SentFolderName = "Sent"

	// TrashFolderName is the default name for the trash folder.
	TrashFolderName = "Trash"

	// DraftsFolderName is the default name for the drafts folder.
	DraftsFolderName = "Drafts"

	// CommandsFolderName is the default name for the folder containing
	// responses to sent commands.
	CommandsFolderName = "Commands"
)
