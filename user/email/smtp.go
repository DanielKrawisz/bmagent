// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

// SMTPConfig contains configuration options for the SMTP server.
type SMTPConfig struct {
	RequireTLS bool
	Username   string
	Password   string
}
