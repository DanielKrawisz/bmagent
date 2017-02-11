// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package user

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"net"
	"net/mail"

	"github.com/DanielKrawisz/bmagent/user/email"
	"github.com/mailhog/data"
	"github.com/mailhog/smtp"
)

// smtpRun handles a smtp session through a tcp connection.
// May be run as a own goroutine if you want to do something else while the
// session runs.
func smtpRun(smtp *smtp.Protocol, conn net.Conn) {
	reader := bufio.NewReader(conn)

	// SMTP begins with a reply code 220.
	reply := smtp.Start()

	// Loop through the pattern of smtp interactions.
	for {
		if reply != nil {
			// Send the latest reply.
			for _, r := range reply.Lines() {
				_, err := conn.Write([]byte(r))
				if err != nil {
					email.SMTPLog.Error(err)
					break
				}
			}
		}

		// Read a line of text from the stream.
		command, err := reader.ReadString([]byte("\n")[0])
		if err != nil {
			break
		}

		// A command is exactly one line of text, so Parse will never return
		// any remaining string we have to worry about.
		_, reply = smtp.Parse(string(command))
	}
}

// SMTPServer provides an SMTP server for handling communications with SMTP
// clients.
type SMTPServer struct {
	cfg *email.SMTPConfig

	// The user to which new messages are to be delivered
	user *User
}

// Serve serves SMTP requests on the given listener.
func (serv *SMTPServer) Serve(l net.Listener) error {
	defer l.Close()
	for {
		conn, err := l.Accept()
		if err != nil {
			return email.SMTPLog.Errorf("Error accepting connection: %s\n", err)
		}

		// Set up the SMTP state machine.
		smtp := smtp.NewProtocol()
		// TODO add TLS support
		// smtp.RequireTLS = serv.cfg.RequireTLS
		smtp.LogHandler = SMTPLogHandler
		smtp.ValidateSenderHandler = serv.validateSender
		smtp.ValidateRecipientHandler = email.ValidateEmail
		smtp.ValidateAuthenticationHandler = serv.validateAuth
		smtp.GetAuthenticationMechanismsHandler = func() []string { return []string{"PLAIN"} }

		smtp.MessageReceivedHandler = serv.messageReceived

		// Start running the protocol.
		go smtpRun(smtp, conn)
	}
}

// validateAuth authenticates the SMTP client.
func (serv *SMTPServer) validateAuth(mechanism string, args ...string) (*smtp.Reply, bool) {
	if mechanism != "PLAIN" {
		return smtp.ReplyUnsupportedAuth(), false
	}

	b, err := base64.StdEncoding.DecodeString(args[0])
	if err != nil {
		return smtp.ReplyError(errors.New("Invalid BASE64 encoding")), false
	}
	s := bytes.Split(b, []byte{0x00})
	if len(s) != 3 {
		return smtp.ReplyInvalidAuth(), false
	}
	user := string(s[1])
	pass := string(s[2])
	// TODO Use time constant comparisons.
	if user != serv.cfg.Username || pass != serv.cfg.Password {
		return smtp.ReplyInvalidAuth(), false
	}
	return smtp.ReplyAuthOk(), true
}

// validateSender validates an email FROM header entry
func (serv *SMTPServer) validateSender(from string) bool {
	addr, err := mail.ParseAddress(from)
	if err != nil {
		return false
	}

	bmAddr, err := email.ToBm(addr.Address)
	if err != nil {
		return false
	}

	if serv.user.keys.Get(bmAddr) == nil {
		return false
	}
	return true
}

// SMTPLogHandler handles logging for the SMTP protocol.
func SMTPLogHandler(message string, args ...interface{}) {
	email.SMTPLog.Debugf(message, args...)
}

// messageReceived is called for each message recieved by the SMTP server.
func (serv *SMTPServer) messageReceived(smtpMessage *data.SMTPMessage) (string, error) {
	email.SMTPLog.Trace("Received message from SMTP server.")

	// TODO is this a good host name?
	message := smtpMessage.Parse("bmagent")

	return string(message.ID), serv.user.DeliverFromSMTP(message.Content)
}

// NewSMTPServer returns a new smtp server.
func NewSMTPServer(cfg *email.SMTPConfig, user *User) *SMTPServer {
	// Set the correct log handler.
	data.LogHandler = SMTPLogHandler

	return &SMTPServer{
		cfg:  cfg,
		user: user,
	}
}
