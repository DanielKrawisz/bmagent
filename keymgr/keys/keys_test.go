// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package keys_test

import (
	"encoding/json"
	"testing"

	"github.com/DanielKrawisz/bmagent/keymgr/keys"
	"github.com/DanielKrawisz/bmutil/identity"
)

func TestMasterKey(t *testing.T) {
	var mKey keys.MasterKey

	// Unexpected format
	err := json.Unmarshal([]byte(`{"key": 546}`), &mKey)
	if err == nil {
		t.Errorf("got no error")
	}

	// Unmarshal correct pubkey.
	err = json.Unmarshal([]byte(`"xprv9s21ZrQH143K4MkMGM9s3pRBt2FXPFJRpCBrrhCT4CPffRNWRDAs9ehsM2RWtCwHxwNQuHWyHCDoEz1bTtHwhihLXuU3dot7YpWpJXS8a1m"`),
		&mKey)
	if err != nil {
		t.Error("failed to unmarshal valid MasterKey:", err)
	}

	// Unmarshal invalid pubkey.
	err = json.Unmarshal([]byte(`"xprv1H143K4MkMGM9s3pRBt2FXPFJRpCBrrhCT4CPffRNm"`),
		&mKey)
	if err == nil {
		t.Error("unmarshalling invalid MasterKey succeeded")
	}

	// Marshalling pubkey has already been tested in manager_test.go.
}

func TestPrivateID(t *testing.T) {
	// Unmarshalling errors
	var id keys.PrivateID

	// Invalid format
	err := json.Unmarshal([]byte(`{"address": 4}`), &id)
	if err == nil {
		t.Errorf("got no error")
	}

	// No address/address too short
	err = json.Unmarshal([]byte(`{"somerandomkey": false}`), &id)
	if err == nil {
		t.Errorf("got no error")
	}

	// Invalid address
	err = json.Unmarshal([]byte(`{
      "address":"BM-2cUJvFYHhXpBHyd",
      "disabled":false,
      "encryptionKey":"5KUXLFRz2pTfKT9ThC2C1Y3MxWRieXq5Fbm2NiBTFbSaeLzmEwp",
      "extraBytes":1000,
      "isChan":true,
      "nonceTrialsPerByte":1000,
      "signingKey":"5HuxWwwZjGvB2zCXRAYVyUiNfavskWKCGxGwtyGCdPZPUQ22YJM"
    }`), &id)
	if err == nil {
		t.Errorf("got no error")
	}

	// Invalid signing key
	err = json.Unmarshal([]byte(`{
      "address":"BM-2cUJvFYHhXpBHyd96KHfjxsgTYi44BajdE",
      "disabled":false,
      "encryptionKey":"5KUXLFRz2pTfKT9ThC2C1Y3MxWRieXq5Fbm2NiBTFbSaeLzmEwp",
      "extraBytes":1000,
      "isChan":true,
      "nonceTrialsPerByte":1000,
      "signingKey":"5HuxWwwZjGvB2zCXRAYVyUiNfavskZPUQ22YJM"
    }`), &id)
	if err == nil {
		t.Errorf("got no error")
	}

	// Invalid encryption key
	err = json.Unmarshal([]byte(`{
      "address":"BM-2cUJvFYHhXpBHyd96KHfjxsgTYi44BajdE",
      "disabled":false,
      "encryptionKey":"5KUXLFRz2pTfKT9ThCbSaeLzmEwp",
      "extraBytes":1000,
      "isChan":true,
      "nonceTrialsPerByte":1000,
      "signingKey":"5HuxWwwZjGvB2zCXRAYVyUiNfavskWKCGxGwtyGCdPZPUQ22YJM"
    }`), &id)
	if err == nil {
		t.Errorf("got no error")
	}

	// Marshalling error, invalid address
	// This test fails because the address cannot be encoded (unknown version,
	// stream).
	ids, _ := identity.NewDeterministic("privacy", 1, 1)
	privacyChan := &keys.PrivateID{
		Private: *ids[0],
		IsChan:  true,
	}
	_, err = json.Marshal(privacyChan)
	if err == nil {
		t.Errorf("got no error")
	}
}
