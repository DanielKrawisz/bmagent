package email_test

import (
	"testing"

	"github.com/monetas/bmclient/email"
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
		bm, err := email.EmailToBM(test.email)
		if !test.valid {
			if err == nil {
				t.Error("Test ", i, " failed; email should not have been accepted:", test.email)
			}
			continue
		}
		if err != nil {
			t.Error("Test ", i, " failed; email should have been accepted:", test.email)
			continue
		}
		address := email.BMToEmail(bm)
		if test.email != address {
			t.Error("Test ", i, " failed reconversion; have ", address, "; want ", test.email)
		}
	}
}
