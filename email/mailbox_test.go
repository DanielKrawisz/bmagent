package email_test

import (
	"testing"

	"github.com/monetas/bmclient/email"
)

func TestGetSequenceNumber(t *testing.T) {
	tests := []struct {
		list     []uint64
		uid      uint64
		sequence uint32
	}{
		{[]uint64{}, 5, 1},
		{[]uint64{39}, 39, 1},
		{[]uint64{39}, 40, 2},
		{[]uint64{39}, 30, 1},
		{[]uint64{7, 9, 20}, 4, 1},
		{[]uint64{7, 9, 20}, 7, 1},
		{[]uint64{7, 9, 20}, 9, 2},
		{[]uint64{7, 9, 20}, 8, 2},
		{[]uint64{7, 9, 20}, 20, 3},
		{[]uint64{7, 9, 20}, 10, 3},
		{[]uint64{7, 9, 20}, 19, 3},
		{[]uint64{7, 9, 20}, 21, 4},
		{[]uint64{7, 12, 16, 18, 19, 20}, 18, 4},
	}

	for i, test := range tests {
		got := email.GetSequenceNumber(test.list, test.uid)

		if got != test.sequence {
			t.Errorf("GetSequenceNumber test case %d; got %d, expected %d.", i, got, test.sequence)
		}
	}
}
