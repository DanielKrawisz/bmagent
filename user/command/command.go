package command

import (
	"github.com/DanielKrawisz/bmutil/format"
)

// Response is the type returned by a command. It can be delivered to
// the user as a json object or as an email.
type Response interface {
	Email() *format.Encoding2
	RPC() interface{}
}
