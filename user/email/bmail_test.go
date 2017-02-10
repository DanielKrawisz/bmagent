// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

import (
	"testing"
)

func TestEmailAddressConversion(t *testing.T) {
	tests := []struct {
		email string
		valid bool
	}{
		// Invalid email address.
		{
			email: "z",
		},
		// Invalid email address format.
		{
			email: "moo@zork.com",
		},
		// Invalid bitmessage address.
		{
			email: "xyz@bm.addr",
		},
		// Invalid bitmessage address.
		{
			email: "BM-NBddNS6ZagzjNbMMkVpecuSAPU1EgyQ@bm.addr",
		},
		// valid.
		{
			email: "BM-NBddNS6ZagzjNbMMkVBpecuSAPU1EgyQ@bm.addr",
			valid: true,
		},
		// A different valid one.
		{
			email: "BM-NBPVwY5A26MtyfbHyh4UfA4Hn76DamAP@bm.addr",
			valid: true,
		},
	}

	for i, test := range tests {
		bm, err := ToBm(test.email)
		if !test.valid {
			if err == nil {
				t.Error("Test ", i, " failed; email should not have been accepted:", test.email)
			}
			continue
		}
		if err != nil {
			t.Error("Test ", i, " failed; email should have been accepted:", test.email, "; err = ", err)
			continue
		}
		address := BmToEmail(bm)
		if test.email != address {
			t.Error("Test ", i, " failed reconversion; have ", address, "; want ", test.email)
		}
	}
}
