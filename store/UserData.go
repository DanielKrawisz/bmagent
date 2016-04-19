// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package store

import (
	"sync"

	"github.com/boltdb/bolt"
)

type UserData struct {
	masterKey          *[keySize]byte      // can be nil.
	db                 *bolt.DB
	bucketId           []byte				// The name of the user's bucket
	username           string				// the username.
	BroadcastAddresses *BroadcastAddresses
	mutex              sync.RWMutex        // For protecting the map.
	folders            map[string]struct{}
}

// NewFolder creates a new mailbox. Name must
// be unique.
func (u *UserData) NewFolder(name string) (*Folder, error) {

	// Check if mailbox exists.
	_, ok := u.folders[name]
	if ok {
		return nil, ErrDuplicateMailbox
	}

	// We're good, so create the mailbox.
	// Save mailbox in the local map.
	u.mutex.Lock()
	defer u.mutex.Unlock()
	
	u.folders[name] = struct{}{}
	
	folder, err := newFolder(u.masterKey, u.db, u.username, name)
	if err != nil {
		return nil, err
	}
	err = folder.initialize()
	if err != nil {
		return nil, err
	}

	return folder, nil
}

// FolderByName retrieves the mailbox associated with the name. If the
// mailbox doesn't exist, an error is returned.
func (u *UserData) FolderByName(name string) (*Folder, error) {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	_, ok := u.folders[name]
	if !ok {
		return nil, ErrNotFound
	}
	return newFolder(u.masterKey, u.db, u.username, name)
}

// Folders returns a slice containing pointers to all folders 
// for a given user. 
func (u *UserData) Folders() []*Folder {
	
	mboxes := make([]*Folder, len(u.folders))
	
	var index uint32 = 0
	for name, _ := range u.folders {
		folder, err := newFolder(u.masterKey, u.db, u.username, name)
		if err != nil {
			return nil
		}
		mboxes[index] = folder
		index ++
	}
	return mboxes
}

// Delete deletes the folder. Any operations after a Delete are invalid.
func (u *UserData) DeleteFolder(name string) error {
	return u.db.Update(func(tx *bolt.Tx) error {
		// Delete the mailbox and associated data.
		bucket := tx.Bucket(u.bucketId)
		if bucket == nil {
			return ErrNotFound
		}
		
		u.mutex.Lock()
		
		err := bucket.Bucket(mailboxesBucket).DeleteBucket([]byte(name))
		if err != nil {
			return err
		}

		// Delete mailbox from store.
		delete(u.folders, name)
		u.mutex.Unlock()
	
		return nil
	})
}

