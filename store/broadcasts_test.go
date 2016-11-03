// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package store_test

import (
	"io/ioutil"
	"testing"

	"github.com/DanielKrawisz/bmagent/store"
	"github.com/DanielKrawisz/bmutil"
)

func TestBroadcastAddresses(t *testing.T) {
	// Open store.
	f, err := ioutil.TempFile("", "tempstore")
	if err != nil {
		t.Fatal(err)
	}
	fName := f.Name()
	f.Close()

	pass := []byte("password")
	uname := "daniel"
	l, err := store.Open(fName)
	s, _, err := l.Construct(pass)
	if err != nil {
		t.Fatal(err)
	}
	
	u, err := s.NewUser(uname)
	if err != nil {
		t.Fatal(err)
	}

	// Start.
	addr1 := "BM-GtovgYdgs7qXPkoYaRgrLFuFKz1SFpsw"
	addr2 := "BM-BcJfZ82sHqW75YYBydFb868yAp1WGh3v"

	// Remove non-existing address.
	err = u.BroadcastAddresses.Remove(addr1)
	if err != store.ErrNotFound {
		t.Error("Expected ErrNotFound got ", err)
	}

	// Check if ForEach works correctly with no addresses in store.
	counter := 0
	u.BroadcastAddresses.ForEach(func(*bmutil.Address) error {
		counter++
		return nil
	})
	if counter != 0 {
		t.Errorf("For counter expected %d got %d", 0, counter)
	}

	// Add 2 address.
	err = u.BroadcastAddresses.Add(addr1)
	if err != nil {
		t.Error(err)
	}
	err = u.BroadcastAddresses.Add(addr2)
	if err != nil {
		t.Error(err)
	}

	// Check if both addresses have been correctly added.
	counter = 0
	u.BroadcastAddresses.ForEach(func(*bmutil.Address) error {
		counter++
		return nil
	})
	if counter != 2 {
		t.Errorf("For counter expected %d got %d", 2, counter)
	}
}
