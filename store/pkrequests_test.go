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
	"github.com/DanielKrawisz/bmagent/store/data"
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
	s, pk, err := l.Construct([]byte("password"))
	if err != nil {
		t.Fatal(err)
	}

	addr1 := "BM-2DB6AzjZvzM8NkS3HMYWMP9R1Rt778mhN8"
	addr2 := "BM-2DAV89w336ovy6BUJnfVRD5B9qipFbRgmr"

	// Check if a non-existing address returns correct error.
	_, err = pk.LastRequestTime(addr1)
	if err != data.ErrNotFound {
		t.Error("Expected ErrNotFound, got", err)
	}

	// Test adding a new address.
	testNewPKRequest(pk, addr1, 1, t)

	// Check if a non-existing address returns correct error.
	_, err = pk.LastRequestTime(addr2)
	if err != data.ErrNotFound {
		t.Error("Expected ErrNotFound, got", err)
	}

	// Add same address that we got ErrNotFound for before.
	testNewPKRequest(pk, addr2, 1, t)

	// Check if we have the right number of addresses.
	count := 0
	err = pk.ForEach(func(address string, reqCount uint32, lastReqTime time.Time) error {
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
	testNewPKRequest(pk, addr1, 2, t)
	testNewPKRequest(pk, addr1, 3, t)
	testNewPKRequest(pk, addr1, 4, t)
	testNewPKRequest(pk, addr2, 2, t)

	// Remove addresses.
	err = pk.Remove(addr1)
	if err != nil {
		t.Error("Got error", err)
	}
	err = pk.Remove(addr2)
	if err != nil {
		t.Error("Got error", err)
	}

	// Check if they are there anymore.
	_, err = pk.LastRequestTime(addr1)
	if err != data.ErrNotFound {
		t.Error("Expected ErrNotFound, got", err)
	}
	_, err = pk.LastRequestTime(addr2)
	if err != data.ErrNotFound {
		t.Error("Expected ErrNotFound, got", err)
	}

	// Close database.
	err = s.Close()
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(fName)
}

func testNewPKRequest(pk *store.PKRequests, address string,
	expectedCount uint32, t *testing.T) {

	// Add/update address.
	count, err := pk.New(address)
	if err != nil {
		t.Errorf("For address %s got error %v", address, err)
	}
	// Get time and check if it's within an acceptable margin.
	timestamp, err := pk.LastRequestTime(address)
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
