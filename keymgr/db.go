// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package keymgr

import (
	"errors"
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

	// ImportedIDs contains all IDs that weren't derived from the master key.
	// This could include channels or addresses imported from PyBitmessage.
	ImportedIDs []*PrivateID `json:"importedIDs"`

	// DerivedIDs contains all IDs derived from the master key.
	DerivedIDs []*PrivateID `json:"derivedIDs"`

	// NewIDIndex contains the index of the next identity that will be derived
	// according to BIP-BM01.
	NewIDIndex uint32 `json:"newIDIndex"`
}

// init initializes the database struct.
func (db *db) init() {
	db.ImportedIDs = make([]*PrivateID, 0, dbInitSize)
	db.DerivedIDs = make([]*PrivateID, 0, dbInitSize)
}

// checkAndUpgrade checks the version of the database and upgrades it if it
// isn't the latest.
func (db *db) checkAndUpgrade() error {
	if db.Version != 1 {
		return ErrUnknownKeyfileVersion
	}
	return nil
}
