// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

import (
	"fmt"
	"testing"
	"time"

	"github.com/DanielKrawisz/bmagent/message/format"
	"github.com/DanielKrawisz/bmagent/store/mem"
	"github.com/jordwest/imap-server/mailstore"
	"github.com/jordwest/imap-server/types"
)

func TestGetSequenceNumber(t *testing.T) {
	tests := []struct {
		list     MessageSequence
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
		got := test.list.GetSequenceNumber(test.uid)

		if got != test.sequence {
			t.Errorf("GetSequenceNumber test case %d; got %d, expected %d.", i, got, test.sequence)
		}
	}
}

// Used to test different implementations of Mailbox
type TestContext interface {
	T() *testing.T

	// Prepare a mailbox that contains messages with uids as given emails,
	// which is assumed to be a sorted list.
	// The maximum email in the folder will be max(emails, nextID - 1)
	MakeMailbox(name string, emails []uint64, nextID uint64) Mailbox
}

func MakeTestBitmessage(from, to, subject, body string) *Bmail {

	expiration, _ := time.Parse(dateFormat, "Mon Jan 2 15:04:05 -0700 MST 2045")
	return &Bmail{
		From:       from,
		To:         to,
		OfChannel:  false,
		Expiration: expiration,
		Content: &format.Encoding2{
			Subject: subject,
			Body:    body,
		},
	}
}

// Prepare a test mailbox that contains messages with uids as given emails,
// which is assumed to be a sorted list.
// The maximum email in the folder will be max(emails, nextID - 1)
func PrepareTestMailbox(mb Mailbox, emails []uint64, nextID uint64) Mailbox {

	// Must be empty.
	if mb.NextUID() != 1 {
		return nil
	}

	addNew := func(next uint64) error {
		return mb.AddNew(MakeTestBitmessage(
			"BM-From",
			"BM-To",
			"top secret",
			fmt.Sprintf("message %s", next),
		), 0)
	}

	// Create the correct set of messages in it.
	var last uint64
	var next uint64 = 1

	for _, id := range emails {
		last++

		for next <= id {
			addNew(next)

			next++
		}

		for last < id {
			mb.DeleteBitmessageByUID(last)

			last++
		}
	}

	// Do we need to add some more messages to get up to nextID?
	for next < nextID {
		addNew(next)

		next++
	}

	return mb
}

type MessageSetByUIDTestCase struct {
	// The test case number.
	c uint32
	// The set of uids which are filled in this mailbox.
	// Should be sorted.
	emails []uint64
	// The value of nextID in this mailbox. If it is <= any value in
	// emails, then the value of nextID will be the maximum value in
	// emails plus 1.
	nextID uint64
	// The sequence set which will be used
	set string
	// The list of ids expected to be returned by the query.
	expected []uint64
}

// Run a query and return the result.
func (x *MessageSetByUIDTestCase) Run(tc TestContext) []mailstore.Message {
	set, err := types.InterpretSequenceSet(x.set)
	if err != nil {
		return nil
	}

	mailbox := tc.MakeMailbox("test inbox", x.emails, x.nextID)
	if mailbox == nil {
		return nil
	}

	return mailbox.MessageSetByUID(set)
}

// Check whether the result is correct.
func (x *MessageSetByUIDTestCase) Check(set []mailstore.Message, t *testing.T) bool {
	if set == nil {
		t.Error("Nil set given.")
		return false
	}

	m := make(map[uint64]struct{})

	// None may be nil.
	for _, msg := range set {
		if msg == nil {
			t.Error("Nil message found.")
			return false
		}

		id := msg.UID()

		m[uint64(id)] = struct{}{}
	}

	// check that the correct set is included.
	for _, id := range x.expected {
		if _, ok := m[id]; !ok {
			t.Error(x.c, "Missing message ", id)
			return false
		}

		delete(m, id)
	}

	// There should be none leftover.
	if len(m) != 0 {
		t.Error("test case ", x.c, ": ", len(m), "extra messages were returned: ", m)
		return false
	}

	return true
}

func testMessageSetByUID(tc TestContext) {
	tests := []MessageSetByUIDTestCase{
		// A simple degenerate test case.
		MessageSetByUIDTestCase{
			0, []uint64{}, 1, "1:*", []uint64{},
		},
		MessageSetByUIDTestCase{
			1, []uint64{}, 6, "1:*", []uint64{1, 2, 3, 4, 5},
		},
		MessageSetByUIDTestCase{
			2, []uint64{}, 6, "2:4", []uint64{2, 3, 4},
		},
		MessageSetByUIDTestCase{
			3, []uint64{1}, 1, "1:*", []uint64{1},
		},
		MessageSetByUIDTestCase{
			4, []uint64{2}, 1, "1:*", []uint64{2},
		},
		MessageSetByUIDTestCase{
			5, []uint64{2}, 1, "3:*", []uint64{},
		},
		MessageSetByUIDTestCase{
			6, []uint64{1, 3}, 1, "1:*", []uint64{1, 3},
		},
		MessageSetByUIDTestCase{
			7, []uint64{1, 3}, 1, "1:1", []uint64{1},
		},
		MessageSetByUIDTestCase{
			8, []uint64{1, 3}, 1, "1:2", []uint64{1},
		},
		MessageSetByUIDTestCase{
			9, []uint64{1, 3}, 1, "1:3", []uint64{1, 3},
		},
		MessageSetByUIDTestCase{
			10, []uint64{1, 3}, 1, "1", []uint64{1},
		},
		MessageSetByUIDTestCase{
			11, []uint64{1, 3}, 1, "3", []uint64{3},
		},
		MessageSetByUIDTestCase{
			12, []uint64{1, 3}, 1, "*", []uint64{3},
		},
	}

	for _, test := range tests {
		test.Check(test.Run(tc), tc.T())
	}
}

// Implementation of TestContext for mailbox
type mailboxTestContext struct {
	t *testing.T
}

func (tc *mailboxTestContext) T() *testing.T {
	return tc.t
}

func (tc *mailboxTestContext) MakeMailbox(name string, emails []uint64, nextID uint64) Mailbox {

	mb, err := newMailbox(mem.NewFolder(name), make(map[string]string))
	if err != nil {
		fmt.Println("Err constructing mailbox: ", err)
		return nil
	}

	return PrepareTestMailbox(mb, emails, nextID)
}

func TestInterface(t *testing.T) {
	boxes := make([]TestContext, 1)

	boxes[0] = &mailboxTestContext{t: t}

	for _, tc := range boxes {
		testMessageSetByUID(tc)
	}
}
