// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package store

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DanielKrawisz/bmagent/store/data"
	"github.com/boltdb/bolt"
)

// Data keys of a mailbox.
var (
	// folderCreatedOnKey contains the time of creation of mailbox.
	folderCreatedOnKey = []byte("createdOn")
)

type folders struct {
	user    *User
	mutex   sync.RWMutex // For protecting the map.
	folders map[string]data.Folder
}

func newFolders(user *User) (*folders, error) {
	folderNames, err := initializeFolders(user.db, user.username, user.bucketID)
	if err != nil {
		return nil, err
	}

	fld := make(map[string]data.Folder)

	for _, name := range folderNames {
		fld[name] = nil
	}

	return &folders{
		user:    user,
		folders: fld,
	}, nil
}

// NewFolder creates a new mailbox. Name must
// be unique.
func (f *folders) New(name string) (data.Folder, error) {

	// Check if mailbox exists.
	_, ok := f.folders[name]
	if ok {
		return nil, ErrDuplicateMailbox
	}

	// We're good, so create the mailbox.
	// Save mailbox in the local map.
	f.mutex.Lock()
	defer f.mutex.Unlock()

	folder, err := newFolder(f.user.masterKey, f.user.db, f.user.username, name)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}

	f.folders[name] = folder

	return folder, nil
}

// Get retrieves the folder associated with the name. If the
// folder doesn't exist, an error is returned.
func (f *folders) Get(name string) (data.Folder, error) {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	flr, ok := f.folders[name]
	if !ok {
		return nil, data.ErrNotFound
	}
	if flr != nil {
		return flr, nil
	}

	var err error
	flr, err = newFolder(f.user.masterKey, f.user.db, f.user.username, name)
	if err != nil {
		return nil, err
	}
	f.folders[name] = flr

	return flr, nil
}

// Names returns a map containing pointers to all folders
// for a given user.
func (f *folders) Names() []string {

	names := make([]string, 0, len(f.folders))

	for name := range f.folders {
		names = append(names, string(name))
	}

	return names
}

// Delete deletes the folder. Any operations after a Delete are invalid.
func (f *folders) Delete(name string) error {
	return f.user.db.Update(func(tx *bolt.Tx) error {
		// Delete the mailbox and associated data.
		bucket := tx.Bucket(f.user.bucketID)
		if bucket == nil {
			return data.ErrNotFound
		}

		f.mutex.Lock()
		defer f.mutex.Unlock()

		err := bucket.Bucket(foldersBucket).DeleteBucket([]byte(name))
		if err != nil {
			return err
		}

		// Delete mailbox from store.
		delete(f.folders, name)

		return nil
	})
}

// Rename renames a folder.
func (f *folders) Rename(name, newname string) error {
	return f.user.db.Update(func(tx *bolt.Tx) error {
		// Delete the mailbox and associated data.
		bucket := tx.Bucket(f.user.bucketID)
		if bucket == nil {
			return data.ErrNotFound
		}

		f.mutex.Lock()
		defer f.mutex.Unlock()

		folders := bucket.Bucket(foldersBucket)

		oldBucket := folders.Bucket([]byte(name))
		if oldBucket == nil {
			return data.ErrNotFound
		}

		if bucket.Bucket([]byte(newname)) != nil {
			return data.ErrDuplicateID
		}

		// create the new bucket.
		_, err := newFolder(f.user.masterKey, f.user.db, f.user.username, newname)
		if err != nil {
			return err
		}

		// copy all items from oldBucket into newBucket
		newBucket := bucket.Bucket([]byte(newname))
		err = oldBucket.ForEach(func(k, v []byte) error {
			newBucket.Put(k, v)
			return nil
		})
		if err != nil {
			return err
		}

		err = bucket.Bucket(foldersBucket).DeleteBucket([]byte(name))
		if err != nil {
			return err
		}

		// Delete mailbox from store.
		delete(f.folders, name)

		return nil
	})
}

// folder is a folder of messages corresponding to a private identity or
// broadcast. It's designed such that any kind of message (not just text
// based) can be stored in the data store.
type folder struct {
	masterKey *[keySize]byte // can be nil.
	db        *bolt.DB
	userID    []byte
	nextID    uint64
	name      string
}

func newFolder(masterKey *[keySize]byte, db *bolt.DB, username string, name string) (*folder, error) {
	if db == nil {
		return nil, errors.New("Nil database given.")
	}

	if name == "" {
		return nil, errors.New("Folder name cannot be empty.")
	}

	f := &folder{
		masterKey: masterKey,
		db:        db,
		userID:    append(userPrefix, []byte(username)...),
		nextID:    1,
		name:      name,
	}

	err := f.initialize(name)
	if err != nil {
		return nil, err
	}

	return f, nil
}

// initializes a folder.
func (f *folder) initialize(name string) error {
	err := f.db.Update(func(tx *bolt.Tx) error {
		// Get bucket for mailbox.
		bucket := tx.Bucket(f.userID).Bucket(foldersBucket).Bucket([]byte(name))

		if bucket == nil {
			// New mailbox so initialize it.
			folders, err := tx.Bucket(f.userID).CreateBucketIfNotExists(foldersBucket)
			if err != nil {
				return err
			}

			newMailbox, err := folders.CreateBucket([]byte(name))
			if err != nil {
				return err
			}

			data, err := newMailbox.CreateBucket(folderDataBucket)
			if err != nil {
				return err
			}

			now, err := time.Now().MarshalBinary()
			if err != nil {
				return err
			}

			err = data.Put(folderCreatedOnKey, now)
			if err != nil {
				return err
			}

			next := make([]byte, 8)
			binary.BigEndian.PutUint64(next, 1)
			data.Put(folderNextIDKey, next)
		} else {
			data := bucket.Bucket(folderDataBucket)
			f.nextID = binary.BigEndian.Uint64(data.Get(folderNextIDKey))

			/*cursor := bucket.Cursor()

			// Loop from the end, returning the first found match.
			for k, _ := cursor.Last(); k != nil; k, _ = cursor.Prev() {
				if len(k) != 16 { // Don't want to loop over buckets.
					continue
				}

				if f.lastID == 0 {
					f.lastID = binary.BigEndian.Uint64(k[:8])
				}

				sfx := binary.BigEndian.Uint64(k[8:])
				if _, ok := f.lastIDBySuffix[sfx]; !ok {
					f.lastIDBySuffix[sfx] = binary.BigEndian.Uint64(k[:8])
				}
			}*/

		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("Failed to create/load mailbox %s: %v",
			name, err)
	}

	return nil
}

// NextID returns the next index value that will be assigned in the mailbox..
func (f *folder) NextID() uint64 {
	return f.nextID
}

// LastID returns the highest index value in the mailbox.
func (f *folder) LastID() (uint64, map[uint64]uint64) {
	var lastID uint64
	lastIDBySuffix := make(map[uint64]uint64)

	f.db.Update(func(tx *bolt.Tx) error {
		// Get bucket for mailbox.
		bucket := tx.Bucket(f.userID).Bucket(foldersBucket).Bucket([]byte(f.name))

		cursor := bucket.Cursor()

		for k, _ := cursor.First(); k != nil; k, _ = cursor.Next() {
			if len(k) != 16 { // Don't want to loop over buckets.
				continue
			}
			suffix := binary.BigEndian.Uint64(k[8:])

			if v, ok := lastIDBySuffix[suffix]; ok && v > suffix {
				continue
			}

			id := binary.BigEndian.Uint64(k[:8])
			lastIDBySuffix[suffix] = id

			if id < lastID {
				continue
			}

			lastID = id
		}

		return nil
	})

	return lastID, lastIDBySuffix
}

// InsertNewMessage inserts a new message with the specified suffix into a
// new position in the database and returns the new id.
func (f *folder) InsertNewMessage(msg []byte, suffix uint64) (uint64, error) {
	if msg == nil {
		return 0, errors.New("Nil message inserted.")
	}

	enc, err := encrypt(f.masterKey, f.db, msg)
	if err != nil {
		return 0, err
	}

	err = f.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(f.userID)
		if bucket == nil {
			return data.ErrNotFound
		}
		bf := bucket.Bucket(foldersBucket).Bucket([]byte(f.name))

		// Increment folderLatestID.
		idB := make([]byte, 8)
		binary.BigEndian.PutUint64(idB, f.nextID+1)

		err = bf.Bucket(folderDataBucket).Put(folderNextIDKey, idB)
		if err != nil {
			return err
		}

		// Insert message using ID as the first 8 bytes and suffix as the
		// latter 8 bytes of the key.
		k := make([]byte, 16)
		binary.BigEndian.PutUint64(k[:8], f.nextID) // first half
		binary.BigEndian.PutUint64(k[8:], suffix)   // second half

		// Check if a message with the given ID already exists.
		kk, _ := bf.Cursor().Seek(k[:8])
		if kk != nil && len(kk) >= 8 && bytes.Equal(kk[:8], k[:8]) {
			return data.ErrDuplicateID
		}

		return bf.Put(k, enc)
	})
	if err != nil {
		return 0, err
	}

	defer func() {
		f.nextID++
	}()

	return f.nextID, nil
}

// InsertMessage inserts a new message with the specified suffix and id into the
// folder and returns the ID.
func (f *folder) InsertMessage(id uint64, msg []byte, suffix uint64) error {
	if id == 0 || id >= f.nextID {
		return data.ErrInvalidID
	}

	if msg == nil {
		return errors.New("Nil message inserted.")
	}

	enc, err := encrypt(f.masterKey, f.db, msg)
	if err != nil {
		return err
	}

	err = f.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(f.userID)
		if bucket == nil {
			return data.ErrNotFound
		}
		m := bucket.Bucket(foldersBucket).Bucket([]byte(f.name))

		// Insert message using ID as the first 8 bytes and suffix as the
		// latter 8 bytes of the key.
		k := make([]byte, 16)
		binary.BigEndian.PutUint64(k[:8], id)     // first half
		binary.BigEndian.PutUint64(k[8:], suffix) // second half

		// Check if a message with the given ID already exists.
		kk, _ := m.Cursor().Seek(k[:8])
		if kk != nil && len(kk) >= 8 && bytes.Equal(kk[:8], k[:8]) {
			return data.ErrDuplicateID
		}

		return m.Put(k, enc)
	})
	if err != nil {
		return err
	}

	return nil
}

// GetMessage retrieves a message from the folder by its index. It returns the
// suffix and the message. An error is returned if the message with the given
// index doesn't exist in the database.
func (f *folder) GetMessage(id uint64) (uint64, []byte, error) {
	var suffix uint64
	var msg []byte

	var success bool

	idBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(idBytes, id)

	err := f.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(f.userID)
		if bucket == nil {
			return data.ErrNotFound
		}

		bucket = bucket.Bucket(foldersBucket)
		if bucket == nil {
			return data.ErrNotFound
		}

		cursor := bucket.Bucket([]byte(f.name)).Cursor()
		k, v := cursor.Seek(idBytes)

		// Check if the first 8 bytes match the index.
		if k == nil || v == nil || !bytes.Equal(k[:8], idBytes) {
			return data.ErrNotFound
		}

		suffix = binary.BigEndian.Uint64(k[8:])
		msg, success = decrypt(f.masterKey, f.db, v)
		if !success {
			return ErrDecryptionFailed
		}

		return nil
	})
	if err != nil {
		return 0, nil, err
	}

	return suffix, msg, nil
}

// ForEachMessage runs the given function for messages that have IDs between
// lowID and highID with the given suffix. If lowID is 0, it starts from the
// first message. If highID is 0, it returns all messages with id >= lowID with
// the given suffix. If suffix is zero, it returns all messages between lowID
// and highID, irrespective of the suffix. Note that any combination of lowID,
// highID and suffix can be zero for the desired results. Both lowID and highID
// are inclusive.
//
// Suffix is useful for getting all messages of a particular type. For example,
// retrieving all messages with encoding type 2.
//
// The function terminates early if an error occurs and iterates in the
// increasing order of index. Make sure it doesn't take long to execute. DO NOT
// execute any other database operations in it.
func (f *folder) ForEachMessage(lowID, highID, suffix uint64,
	fn func(id, suffix uint64, msg []byte) error) error {
	if highID != 0 && lowID > highID {
		return errors.New("Nice try, son. But lowID cannot be greater than highID.")
	}

	bLowID := make([]byte, 8)
	binary.BigEndian.PutUint64(bLowID, lowID)

	return f.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(f.userID)
		if bucket == nil {
			return data.ErrNotFound
		}
		cursor := bucket.Bucket(foldersBucket).Bucket([]byte(f.name)).Cursor()

		for k, v := cursor.Seek(bLowID); k != nil; k, v = cursor.Next() {
			// We need this safeguard because BoltDB also loops over buckets. We
			// don't want to include the bucket "data" in our query.
			if v == nil {
				continue
			}

			id := binary.BigEndian.Uint64(k[:8])
			sfx := binary.BigEndian.Uint64(k[8:])

			// We already exceeded the highest ID, so stop.
			if highID != 0 && id > highID {
				return nil // We're done with all the looping.
			}

			// The suffix doesn't match so go to next message.
			if suffix != 0 && sfx != suffix {
				continue
			}

			msg, success := decrypt(f.masterKey, f.db, v)
			if !success {
				return ErrDecryptionFailed
			}

			if err := fn(id, sfx, msg); err != nil {
				return err
			}
		}

		return nil
	})
}

// DeleteMessage deletes a message with the given index from the store. An error
// is returned if the message doesn't exist in the store.
func (f *folder) DeleteMessage(id uint64) error {
	if id == 0 || id >= f.nextID {
		return data.ErrInvalidID
	}

	idxBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(idxBytes, id)

	err := f.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(f.userID)
		if bucket == nil {
			return data.ErrNotFound
		}
		bucket = bucket.Bucket(foldersBucket).Bucket([]byte(f.name))
		cursor := bucket.Cursor()
		k, v := cursor.Seek(idxBytes)

		// Check if the first 8 bytes match the index.
		if k == nil || v == nil || !bytes.Equal(k[:8], idxBytes) {
			return data.ErrNotFound
		}

		err := bucket.Delete(k)

		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

// setName changes the name of the folders.
func (f *folder) setName(name string) error {
	if name == "" {
		return errors.New("Name cannot be empty.")
	}

	err := f.db.Update(func(tx *bolt.Tx) error {
		// Check if some other mailbox has the same name.
		err := tx.Bucket(f.userID).Bucket(foldersBucket).ForEach(func(k, _ []byte) error {
			if string(k) == name {
				return ErrDuplicateMailbox
			}
			return nil
		})
		if err != nil {
			return err
		}

		// Create the new mailbox.
		b, err := tx.Bucket(f.userID).Bucket(foldersBucket).CreateBucket([]byte(name))
		if err != nil {
			return err
		}

		// Copy everything.
		oldB := tx.Bucket(f.userID).Bucket(foldersBucket).Bucket([]byte(f.name))
		oldB.ForEach(func(k, v []byte) error {
			if v == nil { // It's a bucket.
				b1, err := b.CreateBucket(k)
				if err != nil {
					return err
				}

				// Copy all keys.
				return oldB.Bucket(k).ForEach(func(k1, v1 []byte) error {
					return b1.Put(k1, v1)
				})
			}
			return b.Put(k, v)
		})

		// Delete old mailbox.
		return tx.Bucket(f.userID).Bucket(foldersBucket).DeleteBucket([]byte(f.name))
	})
	if err != nil {
		return err
	}

	f.name = name
	return nil
}
