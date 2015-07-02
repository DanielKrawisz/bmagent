// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package store_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/monetas/bmclient/store"
)

func TestMailbox(t *testing.T) {
	// Open store.
	f, err := ioutil.TempFile("", "tempstore")
	if err != nil {
		t.Fatal(err)
	}
	fName := f.Name()
	f.Close()

	pass := []byte("password")
	s, err := store.Open(fName, pass)
	if err != nil {
		t.Fatal(err)
	}

	// Start.
	addr := "BM-2cV9RshwouuVKWLBoyH5cghj3kMfw5G7BJ"

	// Try to get a mailbox that doesn't yet exist.
	_, err = s.MailboxByAddress(addr)
	if err != store.ErrNotFound {
		t.Error("Expected ErrNotFound got", err)
	}

	// Try to create a mailbox.
	name := "Primary"
	boxType := store.MailboxPrivate

	mbox, err := s.NewMailbox(addr, boxType, name)
	if err != nil {
		t.Error("Got error", err)
	}

	// Verify if the mailbox was created correctly.
	// Check name.
	testName := mbox.GetName()
	if name != testName {
		t.Errorf("Name, expected %s got %s", name, testName)
	}
	// Check mailbox type.
	testType := mbox.GetType()
	if err != nil {
		t.Error("Got error", err)
	}
	if boxType != testType {
		t.Errorf("Type, expected %v got %v", boxType, testType)
	}

	// Try getting non-existant message.
	_, _, err = mbox.GetMessage(1)
	if err != store.ErrNotFound {
		t.Error("Expected ErrNotFound got", err)
	}

	// Try deleting non-existant message.
	err = mbox.DeleteMessage(1)
	if err != store.ErrNotFound {
		t.Error("Expected ErrNotFound got", err)
	}

	// Try inserting a message.
	testInsertMessage(mbox, []byte("A test message."), 1, 1, t)

	// Try inserting another message with a different suffix.
	testInsertMessage(mbox, []byte("Another message"), 2, 2, t)

	// Close and re-open database to test if we can load mailboxes as intended.
	err = s.Close()
	if err != nil {
		t.Fatal(err)
	}
	s, err = store.Open(fName, pass)
	if err != nil {
		t.Fatal(err)
	}
	mbox, err = s.MailboxByAddress(addr)
	if err != nil {
		t.Fatal(err)
	}

	// Test ForEachMessage.
	counter := 0
	err = mbox.ForEachMessage(func(index uint64, suffix uint64, msg []byte) error {
		counter++
		return nil
	})
	if err != nil {
		t.Error("Got error", err)
	}
	if 2 != counter {
		t.Errorf("For counter expected %d got %d", 2, counter)
	}

	// Test ForEachMessageBySuffix.
	// For suffix 1.
	testForEachMessageBySuffix(mbox, 1, 1, t)

	// For suffix 2.
	testForEachMessageBySuffix(mbox, 2, 1, t)

	// For unknown suffix.
	testForEachMessageBySuffix(mbox, 3, 0, t)

	// Try deleting messages.
	testDeleteMessage(mbox, 1, t)
	testDeleteMessage(mbox, 2, t)

	// Try adding mailbox with a duplicate address.
	_, err = s.NewMailbox(addr, store.MailboxBroadcast, "Name2")
	if err != store.ErrDuplicateMailbox {
		t.Error("Expected ErrDuplicateMailbox got", err)
	}

	// Try adding mailbox with a duplicate name.
	addr1 := "BM-GtovgYdgs7qXPkoYaRgrLFuFKz1SFpsw"
	_, err = s.NewMailbox(addr1, boxType, name)
	if err != store.ErrDuplicateName {
		t.Error("Expected ErrDuplicateName got", err)
	}

	// Make sure that the previous mailbox was added.
	_, err = s.NewMailbox(addr1, store.MailboxBroadcast, name)
	if err != store.ErrDuplicateMailbox {
		t.Error("Expected ErrDuplicateMailbox got", err)
	}

	// Try adding mailbox with a duplicate name but different type.
	addr2 := "BM-2D7YvqcbRSv2j2zXmamTm4C3XGrTkZqdt3"
	_, err = s.NewMailbox(addr2, store.MailboxBroadcast, name)
	if err != nil {
		t.Error("Got error", err)
	}

	// Try deleting mailbox.
	err = mbox.Delete()
	if err != nil {
		t.Error("Got error", err)
	}

	// Check if mailbox was actually removed.
	_, err = s.MailboxByAddress(addr)
	if err != store.ErrNotFound {
		t.Error("Expected ErrNotFound got", err)
	}

	// Close database.
	err = s.Close()
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(fName)

}

func testInsertMessage(mbox *store.Mailbox, msg []byte, suffix uint64,
	expectedID uint64, t *testing.T) {
	// Try inserting a new message.
	id, err := mbox.InsertMessage(msg, suffix)
	if err != nil {
		t.Errorf("For message #%d got error %v", expectedID, err)
	}
	if expectedID != id {
		t.Errorf("For message #%[1]d expected id %[1]d got %[2]d", expectedID,
			id)
	}

	// Try retrieving the same message.
	testSuffix, testMsg, err := mbox.GetMessage(id)
	if err != nil {
		t.Errorf("For message #%d got error %v", expectedID, err)
	}
	if suffix != testSuffix {
		t.Errorf("For message #%d expected suffix %d got %d", expectedID,
			suffix, testSuffix)
	}
	if !bytes.Equal(msg, testMsg) {
		t.Errorf(`For message #%d expected "%s" got "%s"`, expectedID,
			string(msg), string(testMsg))
	}
}

func testForEachMessageBySuffix(mbox *store.Mailbox, suffix uint64,
	expectedCount uint64, t *testing.T) {
	counter := uint64(0)
	err := mbox.ForEachMessageBySuffix(suffix, func(index uint64, msg []byte) error {
		counter++
		return nil
	})
	if err != nil {
		t.Errorf("For suffix %d got error %v", suffix, err)
	}
	if expectedCount != counter {
		t.Errorf("For suffix %d expected counter %d got %d", suffix,
			expectedCount, counter)
	}
}

func testDeleteMessage(mbox *store.Mailbox, id uint64, t *testing.T) {
	err := mbox.DeleteMessage(id)
	if err != nil {
		t.Errorf("For id %d got error %v", id, err)
	}
	_, _, err = mbox.GetMessage(id)
	if err != store.ErrNotFound {
		t.Errorf("For id %d got error %v", id, err)
	}
}
