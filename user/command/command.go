package command

import (
	"github.com/DanielKrawisz/bmagent/user/email"
)

const (
	// DefaultStream is set as a constant for now until Bitmessage
	// is popular enough to use more than one stream.
	DefaultStream = 1
)

// Response is the type returned by a command. It can be delivered to
// the user as a json object or as an email.
type Response interface {
	Email() *email.Bmail
	JSON() interface{}
}
