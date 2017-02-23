package cmd

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/DanielKrawisz/bmagent/cmd/rpc"
)

// Key is an element of the patterns used to define command parameters
// for email requests.
type Key uint32

const (
	// KeyRepeated is written +, represents any positive number of repetitions
	// of the previous pattern element.
	KeyRepeated Key = Key(0)

	// KeyRepeatedNull is written * represents any number of repetitions of
	// the previous pattern element, including zero.
	KeyRepeatedNull Key = Key(1)

	// KeyBoolean represents a boolean value.
	KeyBoolean Key = Key(2)

	// KeyNatural represents a natural number value.
	KeyNatural Key = Key(3)

	// KeyString represents a string value.
	KeyString Key = Key(4)

	// KeySymbol represents a symbol value.
	KeySymbol Key = Key(5)
)

var (
	regexNatural = regexp.MustCompile("[0-9]+")
	regexString  = regexp.MustCompile("\\\".*\\\"")
	regexSymbol  = regexp.MustCompile("[a-zA-Z][a-zA-Z0-9]+")
)

func keyString(key Key) string {
	switch key {
	case KeyRepeated:
		return "+"
	case KeyRepeatedNull:
		return "*"
	case KeyBoolean:
		return " boolean"
	case KeyNatural:
		return " natural"
	case KeyString:
		return " string"
	case KeySymbol:
		return " symbol"
	default:
		return ""
	}
}

// Pattern represents a valid way of interpreting a command with a set of
// parameters.
type Pattern struct {
	help  string
	key   []Key
	read  func([]string) (Request, error)
	proto func(*rpc.BMRPCRequest) (Request, error)
}

func patternString(keys []Key) string {
	str := make([]string, len(keys))

	for i, key := range keys {
		str[i] = keyString(key)
	}

	return strings.Join(str, "")
}

func recognizedPatterns(patterns []Pattern) [][]Key {
	key := make([][]Key, len(patterns))
	for i, p := range patterns {
		key[i] = p.key
	}
	return key
}

// ReadPattern attempts to return a type corresponding to the given pattern
// which is read from the given string.
func ReadPattern(params []string, elements ...interface{}) error {
	if len(params) != len(elements) {
		return &ErrInvalidNumberOfParameters{
			params:   params,
			expected: uint32(len(elements)),
		}
	}
	for i, elem := range elements {
		param := params[i]
		switch e := elem.(type) {
		default:
			return ErrInvalidType
		case *bool:
			if param == "true" {
				*e = true
			} else if param == "false" {
				*e = false
			} else {
				return ErrInvalidBoolean
			}
		case *uint64:
			n, err := strconv.ParseUint(param, 10, 64)
			if err != nil {
				return err
			}
			*e = n
		case *string:
			if !regexString.Match([]byte(param)) {
				return ErrInvalidString
			}
			*e = param[1 : len(param)-1]
		}
	}

	return nil
}
