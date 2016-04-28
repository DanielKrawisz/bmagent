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
	folders            map[string]Folder
}

func newUserData(
	masterKey   *[keySize]byte, 
	db          *bolt.DB,
	bucketId    []byte,
	username    string,
	broadcast   *BroadcastAddresses,
	folderNames map[string]struct{}) *UserData {
		
	folders := make(map[string]Folder)
	
	for name, _ := range folderNames {
		folders[name] = nil
	}
	
	return &UserData{
		masterKey : masterKey, 
		db : db, 
		bucketId : bucketId, 
		username : username, 
		BroadcastAddresses: broadcast, 
		folders : folders, 
	}
}

// NewFolder creates a new mailbox. Name must
// be unique.
func (u *UserData) NewFolder(name string) (Folder, error) {

	// Check if mailbox exists.
	_, ok := u.folders[name]
	if ok {
		return nil, ErrDuplicateMailbox
	}

	// We're good, so create the mailbox.
	// Save mailbox in the local map.
	u.mutex.Lock()
	defer u.mutex.Unlock()
	
	folder, err := newFolder(u.masterKey, u.db, u.username, name)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	
	u.folders[name] = folder

	return folder, nil
}

// FolderByName retrieves the mailbox associated with the name. If the
// mailbox doesn't exist, an error is returned.
func (u *UserData) FolderByName(name string) (Folder, error) {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	folder, ok := u.folders[name]
	if !ok {
		return nil, ErrNotFound
	}
	if folder != nil {
		return folder, nil
	}
	
	var err error
	folder, err = newFolder(u.masterKey, u.db, u.username, name)
	if err != nil {
		return nil, err
	}
	u.folders[name] = folder
	
	return folder, nil
}

// Folders returns a slice containing pointers to all folders 
// for a given user. 
func (u *UserData) Folders() []Folder {
	
	mboxes := make([]Folder, len(u.folders))
	
	var index uint32 = 0
	for name, f := range u.folders {
		var folder Folder
		var err error
		
		if f == nil {
			folder, err = newFolder(u.masterKey, u.db, u.username, name)
			if err != nil {
				return nil
			}
		} else {
			folder = f
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
		defer u.mutex.Unlock()
		
		err := bucket.Bucket(foldersBucket).DeleteBucket([]byte(name))
		if err != nil {
			return err
		}

		// Delete mailbox from store.
		delete(u.folders, name)
	
		return nil
	})
}

