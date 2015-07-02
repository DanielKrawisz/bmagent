// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package store_test

import (
	"io/ioutil"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monetas/bmclient/store"
)

func TestPKRequests(t *testing.T) {
	// Open store.
	f, err := ioutil.TempFile("", "tempstore")
	if err != nil {
		t.Fatal(err)
	}
	fName := f.Name()
	f.Close()

	s, err := store.Open(fName, []byte("password"))
	if err != nil {
		t.Fatal(err)
	}

	// Start.
	q := s.PubkeyRequests

	addr1 := "BM-2DB6AzjZvzM8NkS3HMYWMP9R1Rt778mhN8"
	addr2 := "BM-2DAV89w336ovy6BUJnfVRD5B9qipFbRgmr"

	// Check if a non-existing address returns correct error.
	_, err = q.GetTime(addr1)
	if err != store.ErrNotFound {
		t.Error("Expected ErrNotFound, got", err)
	}

	// Test adding a new address.
	testNewPKRequest(q, addr1, t)

	// Check if a non-existing address returns correct error.
	_, err = q.GetTime(addr2)
	if err != store.ErrNotFound {
		t.Error("Expected ErrNotFound, got", err)
	}

	// Add same address that we got ErrNotFound for before.
	testNewPKRequest(q, addr2, t)

	// Check if we have the right number of addresses.
	count := uint32(0)
	var wg sync.WaitGroup
	wg.Add(2)
	err = q.ForEach(func(address string, addTime time.Time) {
		atomic.AddUint32(&count, 1)
		wg.Done()
	})
	if err != nil {
		t.Error("Got error", err)
	}
	wg.Wait()
	if count != 2 {
		t.Errorf("For count, expected %v got %v", 2, count)
	}

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
	_, err = q.GetTime(addr1)
	if err != store.ErrNotFound {
		t.Error("Expected ErrNotFound, got", err)
	}
	_, err = q.GetTime(addr2)
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

func testNewPKRequest(q *store.PKRequests, address string, t *testing.T) {
	// Add new address.
	err := q.New(address)
	if err != nil {
		t.Errorf("For address %s got error %v", address, err)
	}
	// Get time and check if it's within an acceptable margin.
	timestamp, err := q.GetTime(address)
	if err != nil {
		t.Error("Got error", err)
	}
	if d := time.Now().Sub(timestamp); d > time.Millisecond {
		t.Errorf("Expected time to be within 1 ms margin, got %v", d)
	}
}
