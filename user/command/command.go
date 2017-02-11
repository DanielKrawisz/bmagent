package command

import (
	"github.com/DanielKrawisz/bmagent/user/email"
	"github.com/DanielKrawisz/bmutil/identity"
)

const (
	// DefaultStream is set as a constant for now until Bitmessage
	// is popular enough to use more than one stream.
	DefaultStream = 1

	// DefaultBehavior is that acks are returned.
	DefaultBehavior = identity.BehaviorAck
)

// Response is the type returned by a command. It can be delivered to
// the user as a json object or as an email.
type Response interface {
	Email() *email.Bmail
	JSON() interface{}
}
