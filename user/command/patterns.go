package command

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

// Pattern denotes a type of input to a command that can be read as a string.
type Pattern uint32

const (
	// PatternNatural represents a natural number.
	PatternNatural Pattern = Pattern(0)

	// PatternString represents a string delimeted by "
	PatternString Pattern = Pattern(1)
)

var (
	// ErrUnknownPattern is returned when the user provides a pattern type
	// different from those given above.
	ErrUnknownPattern = errors.New("Unknown pattern")

	// ErrInvalidStringPattern is returned when a string is expected but
	// the given input does not match the string pattern.
	ErrInvalidStringPattern = errors.New("Invalid string pattern. Should be delimited by \" ")

	regexNatural = regexp.MustCompile("[0-9]+")
	regexString  = regexp.MustCompile("\\\".*\\\"")
)

// ErrTooManyParameters implements the error interface and represents
// a command given with too many parameters.
type ErrTooManyParameters struct {
	MaxAllowed uint32
}

func (err *ErrTooManyParameters) Error() string {
	return fmt.Sprintf("Too many parameters; expected max %d.", err.MaxAllowed)
}

// ErrUnknown implements the error interface and represents
// an unknown command string.
type ErrUnknown struct {
	Command string
}

func (err *ErrUnknown) Error() string {
	return fmt.Sprintf("Unknown command %s.", err.Command)
}

// ErrValueTooBig implements the error interface and represents
// a command given with too many parameters.
type ErrValueTooBig struct {
	Index uint32
	Value uint64
	Max   uint64
}

func (err *ErrValueTooBig) Error() string {
	return fmt.Sprintf("Parameter %d too big; expected max %d, got %d.", err.Index, err.Max, err.Value)
}

// ReadPattern attempts to return a type corresponding to the given pattern
// which is read from the given string.
func ReadPattern(str string, pattern Pattern) (uint64, string, error) {
	switch pattern {
	default:
		return 0, "", ErrUnknownPattern
	case PatternNatural:
		n, err := strconv.ParseUint(str, 10, 64)
		if err != nil {
			return 0, "", err
		}

		return n, "", nil
	case PatternString:
		if !regexString.Match([]byte(str)) {
			return 0, "", ErrInvalidStringPattern
		}
		return 0, str[1 : len(str)-1], nil
	}
}
