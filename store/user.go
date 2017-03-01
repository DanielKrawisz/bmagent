// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package store

import (
	"github.com/DanielKrawisz/bmagent/store/data"
	"github.com/boltdb/bolt"
)

// User contains all the information relevant for a single user.
type User struct {
	masterKey          *[keySize]byte // can be nil.
	db                 *bolt.DB
	bucketID           []byte // The name of the user's bucket
	username           string // the username.
	BroadcastAddresses *BroadcastAddresses
}

func newUser(
	masterKey *[keySize]byte,
	db *bolt.DB,
	bucketID []byte,
	username string,
	broadcast *BroadcastAddresses) *User {

	return &User{
		masterKey:          masterKey,
		db:                 db,
		bucketID:           bucketID,
		username:           username,
		BroadcastAddresses: broadcast,
	}
}

// Folders returns the email folders for this user.
func (u *User) Folders() (data.Folders, error) {
	return newFolders(u)
}
