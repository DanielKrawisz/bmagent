// Originally derived from: btcsuite/btcd/database/log.go
// Copyright (c) 2013-2015 Conformal Systems LLC.

// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

import (
	"github.com/btcsuite/btclog"
)

// Loggers initialized with no output filters. This means the package will not
// perform any logging by default until the caller requests it.
var (
	IMAPLog btclog.Logger
	SMTPLog btclog.Logger
)

// The default amount of logging is none.
func init() {
	DisableLog()
}

// DisableLog disables all library log output.
func DisableLog() {
	IMAPLog = btclog.Disabled
	SMTPLog = btclog.Disabled
}

// UseIMAPLogger uses a specified Logger to output logging info for the IMAP
// server.
func UseIMAPLogger(logger btclog.Logger) {
	IMAPLog = logger
}

// UseSMTPLogger uses a specified Logger to output logging info for the SMTP
// server.
func UseSMTPLogger(logger btclog.Logger) {
	SMTPLog = logger
}
