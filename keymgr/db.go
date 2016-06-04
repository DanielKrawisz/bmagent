// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package keymgr

import (
	"errors"
	"encoding/json"
	"io"
)

// dbInitSize is the size of the ImportedIDs and DerivedIDs slices.
const dbInitSize = 10

var (
	// ErrUnknownKeyfileVersion  is returned when the version of the keyfile is
	// unknown. Maybe we are using a newer keyfile with an older bmclient.
	ErrUnknownKeyfileVersion = errors.New("unknown keyfile version")
)

// db is a representation of how data should be encoded.
type db struct {	
	// Version is the version of key manager file.
	Version int `json:"version"`
	
	// MasterKey is used to derive all HD encryption and signing keys.
	MasterKey *MasterKey `json:"masterKey"`

	// NewIDIndex contains the index of the next identity that will be derived
	// according to BIP-BM01.
	NewIDIndex uint32 `json:"newIDIndex"`
	
	// IDs maps addresses to private ids. 
	IDs map[string]*PrivateID `json:"addresses"`
}

// newDb returns a new db.
func newDb(key *MasterKey, v int) *db {
	return &db{
		MasterKey : key, 
		Version : v, 
		IDs : make(map[string]*PrivateID),
	}
}

func openDb(r io.Reader) (*db, error) {
	db := &db{
		IDs : make(map[string]*PrivateID),
	}
	
	err := json.NewDecoder(r).Decode(db)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func (db *db) Serialize() ([]byte, error) {
	return json.Marshal(db)
}

// checkAndUpgrade checks the version of the database and upgrades it if it
// isn't the latest.
func (db *db) checkAndUpgrade() error {
	if db.Version != 1 {
		return ErrUnknownKeyfileVersion
	}
	return nil
}
