// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package keymgr_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/DanielKrawisz/bmagent/keymgr"
	"github.com/DanielKrawisz/bmagent/keymgr/keys"
	"github.com/DanielKrawisz/bmutil/identity"
	"github.com/DanielKrawisz/bmutil/pow"
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
	gen1 := mgr.New("Seven Anvil", 1, 0)
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
	gen2 := mgr.New("Intelligent Glue", 1, 0)
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
	privacyChan := &keys.PrivateID{
		Private: identity.NewPrivateID(identity.NewPrivateAddress(ids[0], 4, 1), 0, &pow.Default),
		IsChan:  true,
		Name:    "Hyperluminous",
	}

	mgr.ImportIdentity(*privacyChan)
	if n := mgr.NumDeterministic(); n != 2 {
		t.Errorf("invalid latestIDIndex for no identity, expected %d got %d",
			1, n)
	}
	if n := mgr.NumImported(); n != 1 {
		t.Errorf("invalid numImported for no identity, expected %d got %d",
			1, n)
	}

	// Try to retrieve private identity from address.
	privacyAddr := privacyChan.Address()
	privacyRetrieved := mgr.Get(privacyAddr.String())
	if privacyRetrieved == nil {
		t.Errorf("Get returned nil address")
	}
	if !bytes.Equal(privacyRetrieved.Private.Address().RipeHash()[:], privacyChan.Private.Address().RipeHash()[:]) {
		t.Errorf("got different ripe, expected %v got %v",
			privacyChan.Private.Address().RipeHash(), privacyRetrieved.Private.Address().RipeHash())
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
		t.Errorf("invalid number of deterministic keys, expected %d got %d",
			mgr.NumDeterministic(), mgr1.NumDeterministic())
	}

	// Remove an imported key and check if the operation is successful.
	/*mgr1.RemoveImported(privacyChan.Address())
	if n := mgr1.NumImported(); n != 0 {
		t.Errorf("invalid numImported for no identity, expected %d got %d",
			0, n)
	}
	err = mgr1.ForEach(func(id keymgr.PrivateID) error {
		if bytes.Equal(id.Tag(), privacyChan.Tag()) {
			return errors.New("should not happen")
		}
		return nil
	})
	if err != nil {
		t.Error("imported key not removed from database")
	}*/

	// Try to remove a key that doesn't exist in the database.
	// Should not crash the program. (function removed)
	//mgr1.RemoveImported(privacyChan.Address())

	// Try to retrieve non-existant private identity from address.
	privacyRetrieved = mgr1.Get("BM-2cUfDTJXLeMxAVe7pWXBEneBjDuQ783VSq")
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

// Import a key file from pybitmessage or bmagent.
func testImportKeyFile(t *testing.T, testID int, file string, addresses map[string]string) {
	// First load the test file.
	b, err := ioutil.ReadFile(file)
	if err != nil {
		t.Error(testID, err)
		return
	}

	// Create new key manager.
	seed := []byte(fmt.Sprintf("Another secure seed: %d", testID))
	mgr, err := keymgr.New(seed)
	if err != nil {
		t.Error(testID, err)
		return
	}

	// Finally import the test file.
	keys := mgr.ImportKeys(b)
	if keys == nil {
		t.Error(testID, "could not read keys!")
		return
	}

	// Test that the expected values are there.
	for addr, name := range addresses {
		if n, ok := keys[addr]; !ok {
			if n != name {
				t.Error(testID, " for address ", addr, " found name ", n, " but expected ", name)
			}
		} else {
			t.Error(testID, " could not find key ", addr)
		}
	}
}

func TestImportKeyFile(t *testing.T) {
	expected := map[string]string{
		"BM-2cUfDTJXLeMxAVe7pWXBEneBjDuQ783VSq": "unused deterministic address",
		"BM-2cVauKd3h2FK3dK1FVjahhAk3n95qnVWEU": "unused deterministic address",
		"BM-2cUyqf27vaFmc723VWZhFwdqpQBqEfKtQ2": "Green Jimmy",
		"BM-NB1QmZrQSEtJpqLPAMigQNdi5iMiuXQW":   "Copenhagen Sun Tsu",
		"BM-2cX8ubaVMkrTuAkSNtPgUDYUERbG3gSmYh": "Nothing could be more true.",
	}

	testCases := []struct {
		file     string
		expected map[string]string
	}{
		{
			file:     "test/keysPyBitmessage_test.dat",
			expected: expected,
		},
		{
			file:     "test/keysBmagent_test.dat",
			expected: expected,
		},
	}

	for i, test := range testCases {
		testImportKeyFile(t, i, test.file, nil)
	}
}
