// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email_test

import (
	"testing"

	"github.com/monetas/bmclient/email"
)

func TestGetContentType(t *testing.T) {
	tests := []struct {
		input      string
		acceptible bool
	}{
		{
			input:      "text/plain",
			acceptible: true,
		},
		{
			input:      `text/plain; charset="UTF-8"`,
			acceptible: true,
		},
		{
			input:      "spoon/plain",
			acceptible: false,
		},
		{
			input:      "text/pl@in",
			acceptible: false,
		},
		{
			input:      "X-thingy/betamax",
			acceptible: false,
		},
		{
			input:      `text/plain; a=b; q="c"`,
			acceptible: true,
		},
	}

	for i, test := range tests {
		_, _, _, err := email.TstGetContentType(test.input)
		if (err == nil) && !test.acceptible {
			t.Error("Test ", i, " failed because it should not have been accepted:", test.input)
		}
		if (err != nil) && test.acceptible {
			t.Error("Test ", i, " failed because it should have been accepted:", test.input)
		}
	}
}
