// Originally derived from: btcsuite/btcd/database/log.go
// Copyright (c) 2013-2015 Conformal Systems LLC.

// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

import (
	"github.com/btcsuite/btclog"
)

// Loggers initialized with no output filters. This means the package will not
// perform any logging by default until the caller requests it.
var (
	imapLog btclog.Logger
	smtpLog btclog.Logger
)

// The default amount of logging is none.
func init() {
	DisableLog()
}

// DisableLog disables all library log output.
func DisableLog() {
	imapLog = btclog.Disabled
	smtpLog = btclog.Disabled
}

// UseIMAPLogger uses a specified Logger to output logging info for the IMAP
// server.
func UseIMAPLogger(logger btclog.Logger) {
	imapLog = logger
}

// UseSMTPLogger uses a specified Logger to output logging info for the SMTP
// server.
func UseSMTPLogger(logger btclog.Logger) {
	smtpLog = logger
}
