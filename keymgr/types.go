// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package keymgr

import (
	"encoding/json"
	"errors"

	"github.com/btcsuite/btcutil/hdkeychain"
	"github.com/monetas/bmutil"
	"github.com/monetas/bmutil/identity"
)

// MasterKey is the key from which all HD keys are derived. It's an ExtendedKey
// with JSON marshalling/unmarshalling support.
type MasterKey hdkeychain.ExtendedKey

// PrivateID embeds an identity.Private object, adding support for JSON
// marshalling/unmarshalling and other params.
type PrivateID struct {
	identity.Private

	// IsChan tells whether the identity is that of a channel. Based on this,
	// bmclient could figure out whether to prepare/send ack messages or not,
	// and handle this identity separately from others.
	IsChan bool

	// Disabled tells whether the identity has been marked as inactive. This
	// could be either because it cannot be removed (it's an HD identity that
	// we don't want to receive messages for anymore) or we want to store the
	// private keys for an imported identity but not actively listen on it.
	Disabled bool
}

// MarshalJSON marshals the object into JSON. Part of json.Marshaller interface.
func (k *MasterKey) MarshalJSON() ([]byte, error) {
	return json.Marshal((*hdkeychain.ExtendedKey)(k).String())
}

// UnmarshalJSON unmarshals the object from JSON. Part of json.Unmarshaller
// interface.
func (k *MasterKey) UnmarshalJSON(in []byte) error {
	var str string
	err := json.Unmarshal(in, &str)
	if err != nil {
		return err
	}

	key, err := hdkeychain.NewKeyFromString(str)
	if err != nil {
		return err
	}
	*k = MasterKey(*key)
	return nil
}

// MarshalJSON marshals the object into JSON. Part of json.Marshaller interface.
func (id *PrivateID) MarshalJSON() ([]byte, error) {
	addr, err := id.Address.Encode()
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]interface{}{
		"address":            addr,
		"nonceTrialsPerByte": id.NonceTrialsPerByte,
		"extraBytes":         id.ExtraBytes,
		"signingKey":         bmutil.EncodeWIF(id.SigningKey),
		"encryptionKey":      bmutil.EncodeWIF(id.EncryptionKey),
		"isChan":             id.IsChan,
		"disabled":           id.Disabled,
	})
}

// privateIDStore is a struct used for temporarily storing unmarshalled values.
type privateIDStore struct {
	Address            string `json:"address"`
	NonceTrialsPerByte uint64 `json:"nonceTrialsPerByte"`
	ExtraBytes         uint64 `json:"extraBytes"`
	SigningKey         string `json:"signingKey"`
	EncryptionKey      string `json:"encryptionKey"`
	IsChan             bool   `json:"isChan"`
	Disabled           bool   `json:"disabled"`
}

// UnmarshalJSON unmarshals the object from JSON. Part of json.Unmarshaller
// interface.
func (id *PrivateID) UnmarshalJSON(in []byte) error {
	stored := &privateIDStore{}
	err := json.Unmarshal(in, stored)
	if err != nil {
		return err
	}

	if len(stored.Address) < 4 {
		return errors.New("address too short")
	}
	addr, err := bmutil.DecodeAddress(stored.Address)
	if err != nil {
		return err
	}

	signKey, err := bmutil.DecodeWIF(stored.SigningKey)
	if err != nil {
		return err
	}

	encKey, err := bmutil.DecodeWIF(stored.EncryptionKey)
	if err != nil {
		return err
	}

	id.Address = *addr
	id.SigningKey = signKey
	id.EncryptionKey = encKey
	id.NonceTrialsPerByte = stored.NonceTrialsPerByte
	id.ExtraBytes = stored.ExtraBytes
	id.IsChan = stored.IsChan
	id.Disabled = stored.Disabled

	return nil
}
