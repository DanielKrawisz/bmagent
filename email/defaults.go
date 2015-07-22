// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

const welcomeMsg = `
Welcome to bmclient and to the anonymous, encrypted world of Bitmessage! You are
using version 0.1 alpha.

If you wish to send a bitmessage, just write an email with an address of the
form <destination bitmessage address>@bm.addr. For sending out a broadcast,
shoot an e-mail to broadcast@bm.addr. Don't forget to add the From addresses
in your e-mail client.

Have a bug to report? Care to help out? Please see our github repo:
https://github.com/monetas/bmclient/`

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

	// defaultTrashFolderName is the default name for the trash folder.
	SentFolderName = "Sent"

	// TrashFolderName is the default name for the trash folder.
	TrashFolderName = "Trash"

	// DraftsFolderName is the default name for the drafts folder.
	DraftsFolderName = "Drafts"

	// BroadcastAddress is the address where all broadcasts must be sent.
	BroadcastAddress = "broadcast@bm.addr"

	// BmclientAddress is the address for controlling bmclient. This could
	// include creating a new identity, importing a pre-existing identity,
	// subscribing to a broadcast address etc.
	BmclientAddress = "bmclient@bm.addr"
)
