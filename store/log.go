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
	log btclog.Logger
)

// The default amount of logging is none.
func init() {
	DisableLog()
}

// DisableLog disables all library log output. Logging output is disabled by
// default until either UseLogger or SetLogWriter are called.
func DisableLog() {
	log = btclog.Disabled
}

// UseLogger uses a specified Logger to output logging info.
// This should be used in preference to SetLogWriter if the caller is also
// using btclog.
func UseLogger(logger btclog.Logger) {
	log = logger
}
