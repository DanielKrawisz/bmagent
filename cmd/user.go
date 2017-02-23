package cmd

import "github.com/DanielKrawisz/bmutil"

// User represents an implementation of lower-level functions
// to be performed as commands are executed.
type User interface {
	NewAddress(tag string, sendAck bool) bmutil.Address
}
