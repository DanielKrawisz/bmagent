// Originally derived from: btcsuite/btcd/database/log.go
// Copyright (c) 2013-2015 Conformal Systems LLC.

// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package store

import (
	"github.com/btcsuite/btclog"
)

// Loggers initialized with no output filters. This means the package will not
// perform any logging by default until the caller requests it.
var (
	clientLog btclog.Logger
	serverLog btclog.Logger
)

// The default amount of logging is none.
func init() {
	DisableLog()
}

// DisableLog disables all library log output. Logging output is disabled by
// default until either UseLogger or SetLogWriter are called.
func DisableLog() {
	clientLog = btclog.Disabled
	serverLog = btclog.Disabled
}

// UseClientLogger uses a specified Logger to output client logging info.
// This should be used in preference to SetLogWriter if the caller is also
// using btclog.
func UseClientLogger(logger btclog.Logger) {
	clientLog = logger
}

// UseServerLogger uses a specified Logger to output client logging info.
// This should be used in preference to SetLogWriter if the caller is also
// using btclog.
func UseServerLogger(logger btclog.Logger) {
	serverLog = logger
}
