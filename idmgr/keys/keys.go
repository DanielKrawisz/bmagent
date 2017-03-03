// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package keys

import (
	"encoding/json"
	"errors"

	. "github.com/DanielKrawisz/bmutil"
	"github.com/DanielKrawisz/bmutil/identity"
	"github.com/DanielKrawisz/bmutil/pow"
	"github.com/DanielKrawisz/bmutil/wire/obj"
	"github.com/btcsuite/btcutil/hdkeychain"
)

// ErrInvalidPrivateID is returned when a PrivateID cannot be marshalled to JSON.
var ErrInvalidPrivateID = errors.New("Invalid Private ID")

// MasterKey is the key from which all HD keys are derived. It's an ExtendedKey
// with JSON marshalling/unmarshalling support.
type MasterKey hdkeychain.ExtendedKey

// PrivateID embeds an identity.Private object, adding support for JSON
// marshalling/unmarshalling and other params.
type PrivateID struct {
	Private *identity.PrivateID

	// IsChan tells whether the identity is that of a channel. Based on this,
	// bmclient could figure out whether to prepare/send ack messages or not,
	// and handle this identity separately from others.
	IsChan bool

	// Disabled tells whether the identity has been marked as inactive. This
	// could be either because it cannot be removed (it's an HD identity that
	// we don't want to receive messages for anymore) or we want to store the
	// private keys for an imported identity but not actively listen on it.
	Disabled bool

	// IsImported says whether the identity is imported or derived.
	Imported bool

	// Name is a name for this id.
	Name string
}

// Manager represents a private key manager.
type Manager interface {
	// Get retrieves the private key given the address.
	Get(address string) *PrivateID

	// New creates a new key.
	New(name string, stream uint64, behavior uint32) *PrivateID

	// NewUnnamed creates a new key with no name.
	NewUnnamed(stream uint64, behavior uint32) *PrivateID

	// Names returns the map of addresses to names.
	Names() map[string]string
}

// Address generates the bitmessage address.
func (p *PrivateID) Address() Address {
	return p.Private.Address()
}

// Data returns the PubKeyData object from the PrivateID.
func (p *PrivateID) Data() *obj.PubKeyData {
	return p.Private.Data()
}

// Public generates the Public id from a PrivateID.
func (p *PrivateID) Public() identity.Public {
	return p.Public()
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
func (p *PrivateID) MarshalJSON() ([]byte, error) {
	if p.Private == nil {
		return nil, ErrInvalidPrivateID
	}

	address := p.Address()
	if address == nil {
		return nil, ErrInvalidPrivateID
	}
	addr := address.String()
	pk := p.Private.PrivateKey()
	pow := p.Private.Pow()

	return json.Marshal(map[string]interface{}{
		"address":            addr,
		"nonceTrialsPerByte": pow.NonceTrialsPerByte,
		"extraBytes":         pow.ExtraBytes,
		"signingKey":         EncodeWIF(pk.Signing),
		"encryptionKey":      EncodeWIF(pk.Decryption),
		"isChan":             p.IsChan,
		"disabled":           p.Disabled,
		"imported":           p.Imported,
		"name":               p.Name,
		"behavior":           p.Private.Behavior(),
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
	Imported           bool   `json:"imported"`
	Name               string `json:"name"`
	Behavior           uint32 `json:"behavior"`
}

// UnmarshalJSON unmarshals the object from JSON. Part of json.Unmarshaller
// interface.
func (p *PrivateID) UnmarshalJSON(in []byte) error {
	stored := &privateIDStore{}
	err := json.Unmarshal(in, stored)
	if err != nil {
		return err
	}

	pa, err := identity.ImportWIF(stored.Address, stored.SigningKey, stored.EncryptionKey)
	if err != nil {
		return err
	}

	p.Private = identity.NewPrivateID(pa, stored.Behavior, &pow.Data{
		NonceTrialsPerByte: stored.NonceTrialsPerByte,
		ExtraBytes:         stored.ExtraBytes,
	})

	p.IsChan = stored.IsChan
	p.Disabled = stored.Disabled
	p.Imported = stored.Imported
	p.Name = stored.Name

	return nil
}
