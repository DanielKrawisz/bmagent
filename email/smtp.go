// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

import (
	"bufio"
	"errors"
	"net"
	"net/mail"
	"time"

	"github.com/jordwest/imap-server/types"
	"github.com/mailhog/data"
	"github.com/mailhog/smtp"
	"github.com/monetas/bmclient/keymgr"
	"github.com/monetas/bmclient/store"
	"github.com/monetas/bmutil/pow"
	"github.com/monetas/bmutil/wire"
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
	//keys keymgr.Manager

	// The mailbox in which new messages are to be inserted.
	// TODO come up with a more flexible policy as to where the message
	// can go eventually.
	outbox *Mailbox

	addressBook keymgr.AddressBook

	powQueue *store.PowQueue
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
	// TODO Use time constant comparisons.
	if user != serv.cfg.Username || pass != serv.cfg.Password {
		return smtp.ReplyInvalidAuth(), false
	}
	return smtp.ReplyAuthOk(), true
}

// validateRecipient validates whether the recipient is a valid recipient.
func (serv *SMTPServer) validateRecipient(to string) bool {
	addr, err := mail.ParseAddress(to)
	if err != nil {
		return false
	}
	switch addr.Address {
	case BroadcastAddress:
		return true
	case BmclientAddress:
		return true
	default:
		to, err = EmailToBM(to)
		if err != nil {
			return false
		}
		return true
	}
}

// validateSender validates whether the recipient is a valid sender. For a
// sender to be valid, we must hold the private keys of the sender's Bitmessage
// address.
func (serv *SMTPServer) validateSender(from string) bool {
	addr, err := mail.ParseAddress(from)
	if err != nil {
		return false
	}

	var bmAddr string
	switch addr.Address {
	case BmclientAddress:
		return true
	default:
		bmAddr, err = EmailToBM(addr.Address)
		if err != nil {
			return false
		}
	}

	_, err = serv.addressBook.LookupPrivateIdentity(bmAddr)
	if err != nil {
		return false
	}
	return true
}

// logHandler handles logging for the SMTP protocol.
func (serv *SMTPServer) logHandler(message string, args ...interface{}) {
	smtpLog.Debugf(message, args...)
}

// messageReceived is called for each message recieved by the SMTP server.
func (serv *SMTPServer) messageReceived(message *data.Message) (string, error) {
	smtpLog.Info("Message received:", message.Content.Body)

	// Convert to bitmessage.
	bm, err := NewBitmessageFromSMTP(message.Content)
	if err != nil {
		return "", err
	}

	// Attempt to generate the wire.Object form of the message.
	obj, nonceTrials, extraBytes, err := bm.GenerateObject(serv.addressBook)
	if err != nil {
		return "", err
	}

	// If we were able to generate the object, put it in the pow queue.
	if obj != nil {
		encoded := wire.EncodeMessage(obj)
		target := pow.CalculateTarget(uint64(len(encoded)),
			uint64(obj.ExpiresTime.Sub(time.Now()).Seconds()), nonceTrials, extraBytes)
		_ /*index*/, err := serv.powQueue.Enqueue(target, encoded[8:])
		if err != nil {
			return "", err
		}
	}

	// Put the message in outbox.
	serv.outbox.AddNew(bm, types.FlagRecent&types.FlagSeen)

	return string(message.ID), nil
}

// NewSMTPServer returns a new smtp server.
func NewSMTPServer(cfg *SMTPConfig) *SMTPServer {
	return &SMTPServer{
		cfg: cfg,
	}
}

// getSMTPBody return the body of an e-mail to be delivered through SMTP.
func getSMTPBody(email *data.Content) (string, error) {
	if version, ok := email.Headers["MIME-Version"]; ok {
		if version[0] != "1.0" {
			return "", errors.New("Unrecognized MIME version")
		}

		if contentType, ok := email.Headers["Content-Type"]; !ok {
			return "", errors.New("Unrecognized MIME version")
		} else if contentType[0] != "text/plain" {
			// TODO we should be able to support html bodies.
			return "", errors.New("Unsupported Content-Type; use text/plain")
		} else {
			return email.MIME.Parts[0].Body, nil
		}
	}

	return email.Body, nil
}
