// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"regexp"

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

	// The user to which new messages are to be delivered
	user *User
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
		// TODO put this back in.
		//smtp.ValidateAuthenticationHandler = serv.validateAuth
		smtp.GetAuthenticationMechanismsHandler = func() []string { return []string{"PLAIN"} }

		smtp.MessageReceivedHandler = serv.messageReceived

		// Start running the protocol.
		go smtpRun(smtp, conn)
	}
}

// validateAuth authenticates the SMTP client.
func (serv *SMTPServer) validateAuth(mechanism string, args ...string) (*smtp.Reply, bool) {
	fmt.Println("validateAuth. mechanism: ", mechanism, "; args:", args)
	smtpLog.Trace("validateAuth. mechanism: ", mechanism)
	smtpLog.Trace("validateAuth. args:", args)
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

	_, err = serv.user.addressBook.LookupPrivateIdentity(bmAddr)
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
	smtpLog.Info("messageReceived.Message received:", message.Content.Body)

	// Convert to bitmessage.
	bm, err := NewBitmessageFromSMTP(message.Content)
	if err != nil {
		smtpLog.Error("messageReceived. unable to parse message:", err)
		return "", err
	}

	return string(message.ID), serv.user.DeliverFromSMTP(bm)
}

// NewSMTPServer returns a new smtp server.
func NewSMTPServer(cfg *SMTPConfig, user *User) *SMTPServer {
	return &SMTPServer{
		cfg:  cfg,
		user: user,
	}
}

var (
	contentTypeToken = `[^ \(\)<>@,;:\\\"/\[\]\?\.=[:cntrl:]]+`

	contentTypeType = fmt.Sprintf("(?:application|audio|image|message|multipart|text|video|x\\-%s)", contentTypeToken)

	contentTypeValue = fmt.Sprintf("(?:\\\"[^\\\"]*\\\"|%s)", contentTypeToken)

	contentTypeRegex = regexp.MustCompile(
		fmt.Sprintf("^\\s?(%s)\\s?/\\s?(%s)(?:\\s?;\\s?(%s)=(%s))*\\s?$",
			contentTypeType, contentTypeToken, contentTypeToken, contentTypeValue))
)

// getContentType takes a string representing the a Content-Type email
// header value and parses it into a set of values.
//
// According to http://www.w3.org/Protocols/rfc1341/4_Content-Type.html,
// the proper format for Content-Type is given by
//   Content-Type := type "/" subtype *[";" parameter]
//
//   type :=   "application"     / "audio"
//             / "image"           / "message"
//             / "multipart"  / "text"
//             / "video"           / x-token
//
//   x-token := <The two characters "X-" followed, with no
//              intervening white space, by any token>
//
//   subtype := token
//
//   parameter := attribute "=" value
//
//   attribute := token
//
//   value := token / quoted-string
//
//   token := 1*<any CHAR except SPACE, CTLs, or tspecials>
//
//   tspecials :=  "(" / ")" / "<" / ">" / "@"  ; Must be in
//              /  "," / ";" / ":" / "\" / <">  ; quoted-string,
//              /  "/" / "[" / "]" / "?" / "."  ; to use within
//              /  "="                        ; parameter values
func getContentType(contentType string) (content, subtype string, param map[string]string, err error) {
	matches := contentTypeRegex.FindStringSubmatch(contentType)
	if len(matches) < 2 {
		return "", "", nil, errors.New("Cannot parse")
	}

	param = make(map[string]string)
	for i := 3; i+1 < len(matches); i += 2 {
		param[matches[i]] = matches[i+1]
	}

	return matches[1], matches[2], param, nil
}

// getSMTPBody return the body of an e-mail to be delivered through SMTP.
func getSMTPBody(email *data.Content) (string, error) {
	if version, ok := email.Headers["MIME-Version"]; ok {
		if version[0] != "1.0" {
			return "", errors.New("Unrecognized MIME version")
		}

		contentType, ok := email.Headers["Content-Type"]
		if !ok {
			return "", errors.New("Unrecognized MIME version")
		}
		content, subtype, _, err := getContentType(contentType[0])
		if err != nil {
			return "", err
		}

		if content != "text" || subtype != "plain" {
			smtpLog.Tracef("content: %s; subtype: %s", content, subtype)
			// TODO we should be able to support html bodies.
			return "", errors.New(fmt.Sprint("Unsupported Content-Type:", contentType[0], "; use text/plain instead"))
		}

		//return email.MIME.Parts[0].Body, nil
		return email.Body, nil
	}

	return email.Body, nil
}
