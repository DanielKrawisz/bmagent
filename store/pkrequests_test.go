// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package store_test

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/DanielKrawisz/bmagent/store"
)

func TestPKRequests(t *testing.T) {
	// Open store.
	f, err := ioutil.TempFile("", "tempstore")
	if err != nil {
		t.Fatal(err)
	}
	fName := f.Name()
	f.Close()

	l, err := store.Open(fName)
	s, err := l.Construct([]byte("password"))
	if err != nil {
		t.Fatal(err)
	}

	// Start.
	q := s.PubkeyRequests()

	addr1 := "BM-2DB6AzjZvzM8NkS3HMYWMP9R1Rt778mhN8"
	addr2 := "BM-2DAV89w336ovy6BUJnfVRD5B9qipFbRgmr"

	// Check if a non-existing address returns correct error.
	_, err = q.LastRequestTime(addr1)
	if err != store.ErrNotFound {
		t.Error("Expected ErrNotFound, got", err)
	}

	// Test adding a new address.
	testNewPKRequest(q, addr1, 1, t)

	// Check if a non-existing address returns correct error.
	_, err = q.LastRequestTime(addr2)
	if err != store.ErrNotFound {
		t.Error("Expected ErrNotFound, got", err)
	}

	// Add same address that we got ErrNotFound for before.
	testNewPKRequest(q, addr2, 1, t)

	// Check if we have the right number of addresses.
	count := 0
	err = q.ForEach(func(address string, reqCount uint32, lastReqTime time.Time) error {
		count++
		return nil
	})
	if err != nil {
		t.Error("Got error", err)
	}
	if count != 2 {
		t.Errorf("For count, expected %v got %v", 2, count)
	}

	// Test calling New repeatedly; should increment count.
	testNewPKRequest(q, addr1, 2, t)
	testNewPKRequest(q, addr1, 3, t)
	testNewPKRequest(q, addr1, 4, t)
	testNewPKRequest(q, addr2, 2, t)

	// Remove addresses.
	err = q.Remove(addr1)
	if err != nil {
		t.Error("Got error", err)
	}
	err = q.Remove(addr2)
	if err != nil {
		t.Error("Got error", err)
	}

	// Check if they are there anymore.
	_, err = q.LastRequestTime(addr1)
	if err != store.ErrNotFound {
		t.Error("Expected ErrNotFound, got", err)
	}
	_, err = q.LastRequestTime(addr2)
	if err != store.ErrNotFound {
		t.Error("Expected ErrNotFound, got", err)
	}

	// Close database.
	err = s.Close()
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(fName)
}

func testNewPKRequest(q *store.PKRequests, address string,
	expectedCount uint32, t *testing.T) {

	// Add/update address.
	count, err := q.New(address)
	if err != nil {
		t.Errorf("For address %s got error %v", address, err)
	}
	// Get time and check if it's within an acceptable margin.
	timestamp, err := q.LastRequestTime(address)
	if err != nil {
		t.Error("Got error", err)
	}
	if d := time.Now().Sub(timestamp); d > 50*time.Millisecond {
		t.Errorf("Expected time to be within 50 ms margin, got %v", d)
	}
	// Check if count matches.
	if expectedCount != count {
		t.Errorf("For address %s expected count %d got %d", address,
			expectedCount, count)
	}
}
