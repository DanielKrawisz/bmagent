// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

import (
	"bufio"
	"net"

	"github.com/mailhog/data"
	"github.com/mailhog/smtp"
)

// SMTPConfig contains configuration options for the SMTP server.
type SMTPConfig struct {
	RequireTLS bool
	Username   string
	Password   string
}

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
					smtpLog.Error(err)
					break
				}
			}
		}

		// Read a line of text from the stream.
		command, err := reader.ReadString([]byte("\n")[0])
		if err != nil {
			imapLog.Error(err)
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
	cfg *SMTPConfig
}

// Serve serves SMTP requests on the given listener.
func (serv *SMTPServer) Serve(l net.Listener) error {
	defer l.Close()
	for {
		conn, err := l.Accept()
		if err != nil {
			return smtpLog.Errorf("Error accepting connection: %s\n", err)
		}

		// Set up the SMTP state machine.
		smtp := smtp.NewProtocol()
		// TODO add TLS support
		// smtp.RequireTLS = serv.cfg.RequireTLS
		smtp.LogHandler = serv.logHandler
		smtp.ValidateSenderHandler = serv.validateSender
		smtp.ValidateRecipientHandler = serv.validateRecipient
		smtp.ValidateAuthenticationHandler = serv.validateAuth
		smtp.GetAuthenticationMechanismsHandler = func() []string { return []string{"PLAIN"} }

		smtp.MessageReceivedHandler = serv.messageReceived

		// Start running the protocol.
		go smtpRun(smtp, conn)
	}
}

// validateAuth authenticates the SMTP client.
func (serv *SMTPServer) validateAuth(mechanism string, args ...string) (*smtp.Reply, bool) {
	user := args[0]
	pass := args[1]
	if user != serv.cfg.Username || pass != serv.cfg.Password {
		return smtp.ReplyInvalidAuth(), false
	}
	return smtp.ReplyAuthOk(), true
}

// validateRecipient validates whether the recipient is a valid recipient.
func (serv *SMTPServer) validateRecipient(to string) bool {
	// TODO
	return true
}

// validateSender validates whether the recipient is a valid sender. For a
// sender to be valid, we must hold the private keys of the sender's Bitmessage
// address.
func (serv *SMTPServer) validateSender(from string) bool {
	// TODO
	return true
}

// logHandler handles logging for the SMTP protocol.
func (serv *SMTPServer) logHandler(message string, args ...interface{}) {
	smtpLog.Infof(message, args...)
}

// messageReceived is called for each message recieved by the SMTP server.
func (serv *SMTPServer) messageReceived(message *data.Message) (string, error) {
	smtpLog.Info("Message received:", message.Content.Body)

	//serv.account.Deliver(message, types.FlagSeen&types.FlagRecent)

	return string(message.ID), nil
}

// NewSMTPServer returns a new smtp server.
func NewSMTPServer(cfg *SMTPConfig) *SMTPServer {
	return &SMTPServer{
		cfg: cfg,
	}
}
