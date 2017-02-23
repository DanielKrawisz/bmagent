package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DanielKrawisz/bmagent/cmd/rpc"
)

var (
	// ErrInvalidString is returned when a string is expected but
	// the given input does not match the string pattern.
	ErrInvalidString = errors.New("Invalid string pattern. Should be delimited by \" ")

	// ErrInvalidBoolean is returned when a boolean is expected but
	// the given input does not match boolean type.
	ErrInvalidBoolean = errors.New("Boolean should be 'true' or 'false'")

	// ErrInvalidSymbol is returned when a boolean is expected but
	// the given input does not match boolean type.
	ErrInvalidSymbol = errors.New("Boolean should be 'true' or 'false'")

	// ErrInvalidType is returned when the type provided cannot be read.
	ErrInvalidType = errors.New("Cannot read the given type.")
)

// ErrUnknownCommand implements the error interface
// and is returned when a user calls an unknown command.
type ErrUnknownCommand struct {
	Command string
}

// Error constructs the error message as a string.
func (err *ErrUnknownCommand) Error() string {
	return fmt.Sprintf("Unknown command %s.", err.Command)
}

// ErrUnrecognizedPattern implements the error interface and and
// is returned when the user provides an unrecognized pattern.
type ErrUnrecognizedPattern struct {
	command            string
	recognizedPatterns [][]Key
}

// Error constructs the error message as a string.
func (err *ErrUnrecognizedPattern) Error() string {
	patterns := make([]string, len(err.recognizedPatterns))
	for i, k := range err.recognizedPatterns {
		patterns[i] = patternString(k)
	}
	return fmt.Sprintf("Unrecognized pattern for command %s.\nRecognized patterns are \n\t%s",
		err.command, strings.Join(patterns, "\n\t"))
}

func newErrUnrecognizedPattern(command string, pattern []Pattern) *ErrUnrecognizedPattern {
	return &ErrUnrecognizedPattern{
		command:            command,
		recognizedPatterns: recognizedPatterns(pattern),
	}
}

// ErrInvalidNumberOfParameters is an error representing an invalid number
// of parameters.
type ErrInvalidNumberOfParameters struct {
	params   []string
	expected uint32
}

func (err *ErrInvalidNumberOfParameters) Error() string {
	return fmt.Sprintf("Invalid number of parameters in (%s); expected %d", strings.Join(err.params, " "), err.expected)
}

// errorResponse implements the Response interface and represents a
// response to the user that is an error.
type errorResponse struct {
	err error
}

// ErrorResponse creates an errorResponse object.
func ErrorResponse(err error) Response {
	return &errorResponse{
		err: err,
	}
}

func (err *errorResponse) String() string {
	// Create the message.
	return fmt.Sprintf("Error: %s.", err.err.Error())
}

func (err *errorResponse) RPC() *rpc.BMRPCReply {
	return nil // Not yet implemented.
}
