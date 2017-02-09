// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

// IMAPConfig contains configuration options for the IMAP server.
type IMAPConfig struct {
	Username   string
	Password   string
	RequireTLS bool
}
