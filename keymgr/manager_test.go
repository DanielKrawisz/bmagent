// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package keymgr_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/DanielKrawisz/bmagent/keymgr"
	"github.com/DanielKrawisz/bmutil/identity"
)

func TestOperation(t *testing.T) {
	// Initialize a new key manager.
	seed := []byte("a secure psuedorandom seed (clearly not)")
	mgr, err := keymgr.New(seed)
	if err != nil {
		t.Fatal(err)
	}

	// Check that everything is correctly zeroed.
	if n := mgr.NumDeterministic(); n != 0 {
		t.Errorf("invalid latestIDIndex for no identity, expected %d got %d",
			0, n)
	}
	if n := mgr.NumImported(); n != 0 {
		t.Errorf("invalid numImported for no identity, expected %d got %d",
			0, n)
	}

	// Generate first identity and check if everything works as expected.
	gen1 := mgr.NewHDIdentity(1)
	if gen1.IsChan != false {
		t.Error("generated identity not a channel")
	}
	if n := mgr.NumDeterministic(); n != 1 {
		t.Errorf("invalid latestIDIndex for no identity, expected %d got %d",
			1, n)
	}
	if n := mgr.NumImported(); n != 0 {
		t.Errorf("invalid numImported for no identity, expected %d got %d",
			0, n)
	}

	// Generate second identity and check if everything works as expected.
	gen2 := mgr.NewHDIdentity(1)
	if gen2.IsChan != false {
		t.Error("generated identity not a channel")
	}
	if n := mgr.NumDeterministic(); n != 2 {
		t.Errorf("invalid latestIDIndex for no identity, expected %d got %d",
			1, n)
	}
	if n := mgr.NumImported(); n != 0 {
		t.Errorf("invalid numImported for no identity, expected %d got %d",
			0, n)
	}

	// Import a channel and check if it's imported as expected.
	ids, _ := identity.NewDeterministic("privacy", 1, 1)
	privacyChan := &keymgr.PrivateID{
		Private: *ids[0],
		IsChan:  true,
	}
	// Create an address (export fails without this).
	privacyChan.CreateAddress(4, 1)

	err = mgr.ImportIdentity(privacyChan)
	if err != nil {
		t.Fatal(err)
	}
	if n := mgr.NumDeterministic(); n != 2 {
		t.Errorf("invalid latestIDIndex for no identity, expected %d got %d",
			1, n)
	}
	if n := mgr.NumImported(); n != 1 {
		t.Errorf("invalid numImported for no identity, expected %d got %d",
			1, n)
	}

	// Try to import the same private identity again. Should give error.
	privacyCpy := *privacyChan
	err = mgr.ImportIdentity(&privacyCpy)
	if err == nil {
		t.Error("did not get error importing duplicate identity")
	}

	// Try to retrieve private identity from address.
	privacyAddr, _ := privacyChan.Address.Encode()
	privacyRetrieved := mgr.LookupByAddress(privacyAddr)
	if privacyRetrieved == nil {
		t.Errorf("LookupByAddress returned nil address")
	}
	if !bytes.Equal(privacyRetrieved.Address.Ripe[:], privacyChan.Address.Ripe[:]) {
		t.Errorf("got different ripe, expected %v got %v",
			privacyChan.Address.Ripe, privacyRetrieved.Address.Ripe)
	}

	// Save and encrypt the private keys held by the key manager.
	pass := []byte("a very nice and secure password for my keyfile")
	encData, err := mgr.ExportEncrypted(pass)
	if err != nil {
		t.Fatal(err)
	}

	// Create a new key manager from the encrypted data and check if it's like
	// the original.
	mgr1, err := keymgr.FromEncrypted(encData, pass)
	if err != nil {
		t.Fatal(err)
	}
	if mgr.NumImported() != mgr1.NumImported() {
		t.Errorf("invalid number of imported keys, expected %d got %d",
			mgr.NumImported(), mgr1.NumImported())
	}
	if mgr.NumDeterministic() != mgr1.NumDeterministic() {
		t.Errorf("invalid latestIDIndex, expected %d got %d",
			mgr.NumDeterministic(), mgr1.NumDeterministic())
	}

	// Remove an imported key and check if the operation is successful.
	err = mgr1.RemoveImported(privacyChan)
	if err != nil {
		t.Fatal(err)
	}
	if n := mgr1.NumImported(); n != 0 {
		t.Errorf("invalid numImported for no identity, expected %d got %d",
			0, n)
	}
	err = mgr1.ForEach(func(id *keymgr.PrivateID) error {
		if bytes.Equal(id.Tag(), privacyChan.Tag()) {
			return errors.New("should not happen")
		}
		return nil
	})
	if err != nil {
		t.Error("imported key not removed from database")
	}

	// Try to remove a key that doesn't exist in the database. Should give an
	// error.
	err = mgr1.RemoveImported(privacyChan)
	if err == nil {
		t.Error("did not get error removing nonexistent identity")
	}

	// Try to retrieve non-existant private identity from address.
	privacyRetrieved = mgr1.LookupByAddress(privacyAddr)
	if privacyRetrieved != nil {
		t.Errorf("expected nil id")
	}
}

func TestErrors(t *testing.T) {
	// Encrypted data is too small.
	_, err := keymgr.FromEncrypted([]byte{0x00}, []byte{0x00})
	if err == nil {
		t.Error("didn't get error on small ciphertext")
	}

	// Decryption failure.
	_, err = keymgr.FromEncrypted(bytes.Repeat([]byte{0x00}, 100), []byte("pass"))
	if err == nil {
		t.Error("decryption failure did not give error")
	}
}
