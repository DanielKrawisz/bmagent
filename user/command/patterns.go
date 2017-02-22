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
	// PatternBoolean represents a boolean.
	PatternBoolean Pattern = Pattern(0)

	// PatternNatural represents a natural number.
	PatternNatural Pattern = Pattern(1)

	// PatternString represents a string delimeted by "
	PatternString Pattern = Pattern(2)
)

var (
	// ErrUnknownPattern is returned when the user provides a pattern type
	// different from those given above.
	ErrUnknownPattern = errors.New("Unknown pattern")

	// ErrInvalidStringPattern is returned when a string is expected but
	// the given input does not match the string pattern.
	ErrInvalidStringPattern = errors.New("Invalid string pattern. Should be delimited by \" ")

	// ErrInvalidBooleanPattern is returned when a boolean is expected but
	// the given input does not match boolean type.
	ErrInvalidBooleanPattern = errors.New("Boolean should be 'true' or 'false'")

	regexNatural = regexp.MustCompile("[0-9]+")
	regexString  = regexp.MustCompile("\\\".*\\\"")
)

// ErrUnknown implements the error interface
// and is returned when a user calls an unknown command.
type ErrUnknown struct {
	Command string
}

func (err *ErrUnknown) Error() string {
	return fmt.Sprintf("Unknown command %s.", err.Command)
}

// ErrTooManyParameters implements the error interface
// and is returned when a command is given with too many parameters.
type ErrTooManyParameters struct {
	MaxAllowed uint32
}

func (err *ErrTooManyParameters) Error() string {
	return fmt.Sprintf("Too many parameters; expected max %d.", err.MaxAllowed)
}

// ErrValueTooBig implements the error interface and is returned when
// a command is given with a parameter that is too big.
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
func ReadPattern(str string, elem interface{}) error {
	switch e := elem.(type) {
	default:
		return ErrUnknownPattern
	case *bool:
		if str == "true" {
			*e = true
			return nil
		} else if str == "false" {
			*e = false
			return nil
		} else {
			return ErrInvalidBooleanPattern
		}
	case *uint64:
		n, err := strconv.ParseUint(str, 10, 64)
		if err != nil {
			return err
		}
		*e = n
		return nil
	case *string:
		if !regexString.Match([]byte(str)) {
			return ErrInvalidStringPattern
		}
		*e = str[1 : len(str)-1]
		return nil
	}
}
