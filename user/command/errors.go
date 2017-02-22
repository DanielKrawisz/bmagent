package command

import (
	"fmt"
	"github.com/DanielKrawisz/bmutil/format"
)

// errorResponse implements the Response interface and represents a
// response to the user that is an error.
type errorResponse struct {
	cmd    string
	params string
	err    error
}

// ErrorResponse creates an errorResponse object.
func ErrorResponse(cmd, params string, err error) Response {
	return &errorResponse{
		cmd:    cmd,
		params: params,
		err:    err,
	}
}

func (err *errorResponse) Email() *format.Encoding2 {
	// Create the message.
	return &format.Encoding2{
		Subject: fmt.Sprintf("Error response to %s", err.cmd),
		Body: fmt.Sprintf("In response to this command: \n\n\t%s %s\n\nthe following error was generated\n\n\t%s",
			err.cmd, err.params, err.err.Error()),
	}
}

func (err *errorResponse) RPC() interface{} {
	return nil // Not yet implemented.
}
