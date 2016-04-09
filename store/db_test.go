// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package store_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/DanielKrawisz/bmagent/store"
)

func TestOpenClose(t *testing.T) {
	f, err := ioutil.TempFile("", "tempstore")
	if err != nil {
		panic(err)
	}
	fName := f.Name()
	f.Close()

	pass := []byte("password")
	uname := "daniel"
	passNew := []byte("new_password")

	// Create a new database

	l, err := store.Open(fName)
	s, _, _, err := l.Construct(uname, pass)
	if err != nil {
		t.Fatal("Failed to open database:", err)
	}
	err = s.Close()
	if err != nil {
		t.Error("Failed to close database:", err)
	}

	// Try opening same database but with incorrect passphrase
	l, err = store.Open(fName)
	s, _, _, err = l.Construct(uname, passNew)
	if err != store.ErrDecryptionFailed {
		t.Error("Expected ErrDecryptionFailed got", err)
	}

	// Try re-opening database with correct passphrase, to make sure decryption
	// works.
	l, err = store.Open(fName)
	s, _, _, err = l.Construct(uname, pass)
	if err != nil {
		t.Fatal("Failed to open database:", err)
	}

	// Change passphrase and close database.
	err = s.ChangePassphrase(passNew)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Close()
	if err != nil {
		t.Error("Failed to close database:", err)
	}

	// Re-open database with new passphrase to see if ChangePassphrase was
	// successful.
	l, err = store.Open(fName)
	s, _, _, err = l.Construct(uname, passNew)
	if err != nil {
		t.Fatal("Failed to open database:", err)
	}
	err = s.Close()
	if err != nil {
		t.Error("Failed to close database:", err)
	}

	os.Remove(fName)
}

func TestCounters(t *testing.T) {
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
	s, _, _, err := l.Construct(uname, pass)
	if err != nil {
		t.Fatal(err)
	}

	// Start.

	// Try getting counter for when it doesn't exist.
	c, err := s.GetCounter(0)
	if err != nil {
		t.Error(err)
	}
	if 1 != c {
		t.Errorf("For counter expected %d got %d", 1, c)
	}

	// Try setting counter value.
	err = s.SetCounter(0, 34)
	if err != nil {
		t.Error(err)
	}

	// Check if value was saved correctly.
	c, err = s.GetCounter(0)
	if err != nil {
		t.Error(err)
	}
	if 34 != c {
		t.Errorf("For counter expected %d got %d", 34, c)
	}

	// Close database.
	err = s.Close()
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(fName)
}
