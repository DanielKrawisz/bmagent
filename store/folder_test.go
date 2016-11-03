// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package store_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/DanielKrawisz/bmagent/store"
)

func TestFolder(t *testing.T) {
	// Open store.
	f, err := ioutil.TempFile("", "tempstore")
	if err != nil {
		t.Fatal(err)
	}
	fName := f.Name()
	f.Close()

	pass := []byte("password")
	l, err := store.Open(fName)
	s, _, err := l.Construct(pass)
	if err != nil {
		t.Fatal(err)
	}

	uname := "cosmos"
	u, err := s.NewUser(uname)
	if err != nil {
		t.Error(" could not create user: ", err)
	}
	
	// Start.
	name := "INBOX/Test Mailbox"

	// Try to get a mailbox that doesn't yet exist.
	_, err = u.FolderByName(name)
	if err != store.ErrNotFound {
		t.Error("Expected ErrNotFound got", err)
	}

	// Try to create a mailbox.
	mbox, err := u.NewFolder(name)
	if err != nil {
		t.Fatal("Got error", err)
	}

	// Check name.
	testName := mbox.Name()
	if name != testName {
		t.Errorf("Name, expected %s got %s", name, testName)
	}

	// Try getting non-existant message.
	_, _, err = mbox.GetMessage(1)
	if err != store.ErrNotFound {
		t.Error("Expected ErrNotFound got", err)
	}

	// Try getting last IDs when mailbox is empty.
	last, lastIdBySfx := mbox.LastID()
	if last != 0 {
		t.Error("Expected 0 got", last)
	}

	if lastIdBySfx[1] != 0 {
		t.Error("Expected 0 got", lastIdBySfx[1])
	}

	// Try deleting non-existant message.
	err = mbox.DeleteMessage(1)
	if err != store.ErrInvalidID {
		t.Error("Expected ErrInvalidID got", err)
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
	l, err = store.Open(fName)
	s, _, err = l.Construct(pass)
	if err != nil {
		t.Fatal(err)
	}
	u, err = s.GetUser(uname)
	if err != nil {
		t.Fatal(err)
	}
	mbox, err = u.FolderByName(name)
	if err != nil {
		t.Fatal(err)
	}
	
	tc := &boltFolderTestContext{t:t, u:u, name:name}

	// Test ForEachMessage.

	// lowID = 0, highID = 0, suffix = 0 [Get all messages], expectedCount = 2
	testForEachMessage(mbox, 0, 0, 0, 2, t)

	// lowID = 0, highID = 1, suffix = 0, expectedCount = 1
	testForEachMessage(mbox, 0, 1, 0, 1, t)

	// lowID = 2, highID = 5, suffix = 0, expectedCount = 1
	testForEachMessage(mbox, 0, 1, 0, 1, t)

	// lowID = 3, highID = 5, suffix = 0, expectedCount = 0
	testForEachMessage(mbox, 3, 5, 0, 0, t)

	// lowID = 1, highID = 2, suffix = 1, expectedCount = 1
	testForEachMessage(mbox, 1, 2, 1, 1, t)

	// lowID = 1, highID = 2, suffix = 0, expectedCount = 2
	testForEachMessage(mbox, 1, 2, 0, 2, t)

	// lowID = 0, highID = 0, suffix = 1 [Get all messages of suffix 1],
	// expectedCount = 1
	testForEachMessage(mbox, 0, 0, 1, 1, t)

	// lowID = 0, highID = 0, suffix = 2 [Get all messages of suffix 2],
	// expectedCount = 1
	testForEachMessage(mbox, 0, 0, 2, 1, t)

	// lowID = 0, highID = 0, suffix = 3 [Get all messages of suffix 3],
	// expectedCount = 0
	testForEachMessage(mbox, 0, 0, 3, 0, t)

	// Test LastID. Should be 2.
	id, _ := mbox.LastID()
	if id != 2 {
		t.Errorf("Expected %d got %d", 2, id)
	}

	// Verify that the last ID for message with suffix 1 is 1.
	testLastIDBySuffix(tc, mbox, 1, 1, "A")

	// Verify that the last ID for message with suffix 2 is 2.
	testLastIDBySuffix(tc, mbox, 2, 2, "B")

	// Try deleting messages.
	testDeleteMessage(tc, mbox, 1, "C")

	// Check the last ID now. Should still be 2.
	testLastIDBySuffix(tc, mbox, 2, 2, "D")

	// Verify that the last ID for message with suffix 1 that was just deleted
	// is gone too.
	_, lastIdBySfx = mbox.LastID()
	if lastIdBySfx[1] != 0 {
		t.Error("Expected 0 got", lastIdBySfx[1])
	}

	// Verify that the last ID for message with suffix 2 is 2, as expected.
	testLastIDBySuffix(tc, mbox, 2, 2, "E")

	// Delete the last message.
	testDeleteMessage(tc, mbox, 2, "F")

	// LastID should error out.
	last, _ = mbox.LastID()
	if last != 0 {
		t.Error("Expected 0 got", last)
	}

	// Try adding mailbox with a duplicate name.
	_, err = u.NewFolder(name)
	if err != store.ErrDuplicateMailbox {
		t.Error("Expected ErrDuplicateMailbox got", err)
	}

	// Try deleting mailbox.
	err = u.DeleteFolder(name)
	if err != nil {
		t.Error("Got error", err)
	}

	// Check if mailbox was actually removed.
	_, err = u.FolderByName(name)
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

func NewUserData(t *testing.T) *store.UserData {
	// Open store.
	f, err := ioutil.TempFile("", "tempstore")
	if err != nil {
		t.Fatal(err)
	}
	fName := f.Name()
	f.Close()

	pass := []byte("password")
	l, err := store.Open(fName)
	s, _, err := l.Construct(pass)
	if err != nil {
		t.Fatal(err)
	}

	uname := "cosmos"
	u, err := s.NewUser(uname)
	if err != nil {
		t.Error(" could not create user: ", err)
	}

	return u
}

func testInsertMessage(mbox store.Folder, msg []byte, suffix uint64,
	expectedID uint64, t *testing.T) {
	// Try inserting a new message.
	id, err := mbox.InsertNewMessage(msg, suffix)
	if err != nil {
		t.Errorf("For message #%d got error %v", expectedID, err)
	}
	if expectedID != id {
		t.Errorf("For message #%[1]d expected id %[1]d got %[2]d", expectedID,
			id)
	}
	
	last, _ := mbox.LastID()
	next := mbox.NextID()
	if last != id {
		t.Errorf("For message #%[1]d expected id %[1]d got %[2]d", id, last)
	}
	if next != id + 1 {
		t.Errorf("For message #%[1]d expected id %[1]d got %[2]d", id + 1, next)
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

	// Try inserting message with the same ID but different suffix.
	err = mbox.InsertMessage(id, msg, suffix+1)
	if err != store.ErrDuplicateID {
		t.Errorf("For message #%d expected ErrDuplicateID got %v", expectedID, err)
	}
}

func testForEachMessage(mbox store.Folder, lowID, highID, suffix,
	expectedCount uint64, t *testing.T) {

	counter := uint64(0)
	err := mbox.ForEachMessage(lowID, highID, suffix,
		func(index, suffix uint64, msg []byte) error {
			counter++
			return nil
		})
	if err != nil {
		t.Errorf("For lowID %d, highID %d, suffix %d, got error %v", lowID,
			highID, suffix, err)
	}
	if expectedCount != counter {
		t.Errorf("For lowID %d, highID %d, suffix %d, expected counter %d got %d",
			lowID, highID, suffix, expectedCount, counter)
	}
}

func testDeleteMessage(tc testContext, mbox store.Folder, id uint64, note string) {
	err := mbox.DeleteMessage(id)
	if err != nil {		
		tc.T().Errorf("%s, %s: For id %d got error %v", tc.Context(), note, id, err)
	}
	_, _, err = mbox.GetMessage(id)
	if err == nil {
		tc.T().Errorf("%s, %s: for id %d expected ErrNotFound got nil", tc.Context(), note, id)
	} else if err != store.ErrNotFound {
		tc.T().Errorf("%s, %s: For id %d got error %v", tc.Context(), note, id, err)
	}
}

func testLastIDBySuffix(
	tc testContext, mbox store.Folder, suffix, expectedID uint64, note string) {
		
	_, lastIdBySfx := mbox.LastID()
	id  := lastIdBySfx[suffix]
	if expectedID != id {
		tc.T().Errorf("%s, %s: For suffix %d, expected ID %d got %d", 
			tc.Context(), note, suffix, expectedID, id)
	}
}
