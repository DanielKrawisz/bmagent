// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package store

import (
	"bytes"
	"encoding/binary"
	"errors"
	"time"
	"fmt"

	"github.com/boltdb/bolt"
)

// Data keys of a mailbox.
var (
	// folderCreatedOnKey contains the time of creation of mailbox.
	folderCreatedOnKey = []byte("createdOn")

	// ErrDuplicateID is returned by InsertMessage when the a message with the
	// specified ID already exists in the folder.
	ErrDuplicateID = errors.New("duplicate ID")

	// ErrInvaidID is returned when an invalid id is provided. 
	ErrInvalidID = errors.New("invalid ID")
)

type Folder interface {
	// Name returns the user-friendly name of the folder.
	Name() string

	// SetName changes the name of the folder.
	SetName(name string) error 

	// InsertNewMessage inserts a new message with the specified suffix and
	// id into the folder and returns the ID. For normal folders, suffix
	// could be the encoding type. For special use folders like "Pending",
	// suffix could be used as a 'key', like a reason code (why the message is
	// marked as Pending).
	InsertNewMessage(msg []byte, suffix uint64) (uint64, error)

	// InsertMessage inserts a new message with the specified suffix and id
	// into the folder and returns the ID. If input id is 0 or >= NextID,
	// ErrInvalidID is returned. 
	InsertMessage(id uint64, msg []byte, suffix uint64) error

	// GetMessage retrieves a message from the folder by its index. It returns the
	// suffix and the message. An error is returned if the message with the given
	// index doesn't exist in the database.
	GetMessage(id uint64) (uint64, []byte, error)

	// DeleteMessage deletes a message with the given index from the store. An error
	// is returned if the message doesn't exist in the store.
	DeleteMessage(id uint64) error 

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
	ForEachMessage(lowID, highID, suffix uint64,
		fn func(id, suffix uint64, msg []byte) error) error
	
	// NextID returns the next index value that will be assigned in the mailbox..
	NextID() uint64

	// LastID returns the highest index value in the mailbox.
	LastID() uint64

	// LastIDBySuffix returns the highest index value from messages with the
	// specified suffix in the mailbox.
	LastIDBySuffix(suffix uint64) uint64 
}

// folder is a folder of messages corresponding to a private identity or
// broadcast. It's designed such that any kind of message (not just text
// based) can be stored in the data store.
type folder struct {
	masterKey      *[keySize]byte      // can be nil.
	db             *bolt.DB
	username       string
	userId         []byte
	name           string
	nextId         uint64
	lastId         uint64
	lastIdBySuffix map[uint64]uint64
}

func newFolder(masterKey *[keySize]byte, db *bolt.DB, username string, name string) (*folder, error) {
	if db == nil {
		return nil, errors.New("Nil database given.")
	}
	
	if name == "" {
		return nil, errors.New("Folder name cannot be empty.")
	}
	
	f := &folder {
		masterKey      : masterKey,
		db             : db,
		userId         : append(userPrefix, []byte(username)...),  
		username       : username, 
		name           : name, 
		nextId         : 1, 
		// TODO go through the folder and populate these values properly.
		lastId         : 0, 
		lastIdBySuffix : make(map[uint64]uint64), 
	}
	
	err := f.initialize()
	if err != nil {
		return nil, err
	}
	
	return f, nil
}

// initializes a folder.
func (f *folder) initialize() error {

	err := f.db.Update(func(tx *bolt.Tx) error {
		// Get bucket for mailbox.
		bucket := tx.Bucket(f.userId).Bucket(foldersBucket).Bucket([]byte(f.name))

		if bucket == nil {
			// New mailbox so initialize it.
			folders, err := tx.Bucket(f.userId).CreateBucketIfNotExists(foldersBucket)
			if err != nil {
				return err
			}
			
			newMailbox, err := folders.CreateBucket([]byte(f.name))
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
			// TODO get all the folder data here. 
			data := bucket.Bucket(folderDataBucket)
			f.nextId = binary.BigEndian.Uint64(data.Get(folderNextIDKey))
			
			cursor := bucket.Cursor()
	
			// Loop from the end, returning the first found match.
			for k, _ := cursor.Last(); k != nil; k, _ = cursor.Prev() {
				if k == nil {
					return ErrNotFound
				}
				
				if len(k) != 16 { // Don't want to loop over buckets.
					continue
				}
				
				if f.lastId == 0 {
					f.lastId = binary.BigEndian.Uint64(k[:8])
				}
				
				sfx := binary.BigEndian.Uint64(k[8:])
				if _, ok := f.lastIdBySuffix[sfx]; !ok {
					f.lastIdBySuffix[sfx] = binary.BigEndian.Uint64(k[:8])
				}
			}
			
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("Failed to create/load mailbox %s: %v",
			f.name, err)
	}

	return nil
}

// NextID returns the next index value that will be assigned in the mailbox..
func (f *folder) NextID() uint64 {
	return f.nextId
}

// LastID returns the highest index value in the mailbox.
func (f *folder) LastID() uint64 {
	return f.lastId
}

// LastIDBySuffix returns the highest index value from messages with the
// specified suffix in the mailbox.
func (f *folder) LastIDBySuffix(suffix uint64) uint64 {
	if last, ok := f.lastIdBySuffix[suffix]; ok {
		return last
	}
	
	return 0

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
		bucket := tx.Bucket(f.userId)
		if bucket == nil {
			return ErrNotFound
		}
		bf := bucket.Bucket(foldersBucket).Bucket([]byte(f.name))

		// Increment folderLatestID.
		idB := make([]byte, 8)
		binary.BigEndian.PutUint64(idB, f.nextId + 1)

		err = bf.Bucket(folderDataBucket).Put(folderNextIDKey, idB)
		if err != nil {
			return err
		}

		// Insert message using ID as the first 8 bytes and suffix as the
		// latter 8 bytes of the key.
		k := make([]byte, 16)
		binary.BigEndian.PutUint64(k[:8], f.nextId)     // first half
		binary.BigEndian.PutUint64(k[8:], suffix) // second half

		// Check if a message with the given ID already exists.
		kk, _ := bf.Cursor().Seek(k[:8])
		if kk != nil && len(kk) >= 8 && bytes.Equal(kk[:8], k[:8]) {
			return ErrDuplicateID
		}

		return bf.Put(k, enc)
	})
	if err != nil {
		return 0, err
	}
	
	defer func() {
		f.nextId ++
	} ()
	
	f.lastId = f.nextId
	f.lastIdBySuffix[suffix] = f.nextId
	
	return f.nextId, nil
}

// InsertMessage inserts a new message with the specified suffix and id into the
// folder and returns the ID. 
func (f *folder) InsertMessage(id uint64, msg []byte, suffix uint64) error {
	if id == 0 || id >= f.nextId {
		return ErrInvalidID
	}
	
	if msg == nil {
		return errors.New("Nil message inserted.")
	}
	
	enc, err := encrypt(f.masterKey, f.db, msg)
	if err != nil {
		return err
	}

	err = f.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(f.userId)
		if bucket == nil {
			return ErrNotFound
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
			return ErrDuplicateID
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
		bucket := tx.Bucket(f.userId)
		if bucket == nil {
			return ErrNotFound
		}
		
		bucket = bucket.Bucket(foldersBucket)
		if bucket == nil {
			return ErrNotFound
		}
		
		cursor := bucket.Bucket([]byte(f.name)).Cursor()
		k, v := cursor.Seek(idBytes)

		// Check if the first 8 bytes match the index.
		if k == nil || v == nil || !bytes.Equal(k[:8], idBytes) {
			return ErrNotFound
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
		bucket := tx.Bucket(f.userId)
		if (bucket == nil) {
			return ErrNotFound
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
	if id == 0 || id >= f.nextId {
		return ErrInvalidID
	}
	
	idxBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(idxBytes, id)
	
	err := f.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(f.userId)
		if bucket == nil {
			return ErrNotFound
		}
		bucket = bucket.Bucket(foldersBucket).Bucket([]byte(f.name))
		cursor := bucket.Cursor()
		k, v := cursor.Seek(idxBytes)

		// Check if the first 8 bytes match the index.
		if k == nil || v == nil || !bytes.Equal(k[:8], idxBytes) {
			return ErrNotFound
		}

		// Get suffix of entry. 
		suffix := binary.BigEndian.Uint64(k[8:])

		err := bucket.Delete(k)
		
		if err != nil {
			return err
		}
		
		if f.lastIdBySuffix[suffix] != id {
			return nil
		}
	
		for {
			k, _ = cursor.Prev()
			
			// We got to the front of the bucket without finding anything.
			if k == nil {
				delete(f.lastIdBySuffix, suffix)
				
				if id == f.lastId {
					f.lastId = 0
				}
				
				break
			}
			
			if len(k) != 16 { // Don't want to loop over buckets.
				continue
			}
			lastId := binary.BigEndian.Uint64(k[:8])
			
			if f.lastId == id {
				f.lastId = lastId
			}
			
			sfx := binary.BigEndian.Uint64(k[8:])
			
			if sfx == suffix {
					
				f.lastIdBySuffix[suffix] = lastId
						
				break
			}
		}
		
		return nil
	})
	
	if err != nil {
		return err
	}
	
	return nil
}

// Name returns the user-friendly name of the mailbox.
func (f *folder) Name() string {
	return f.name
}

// SetName changes the name of the mailbox.
func (f *folder) SetName(name string) error {
	if name == "" {
		return errors.New("Name cannot be empty.")
	}

	err := f.db.Update(func(tx *bolt.Tx) error {
		// Check if some other mailbox has the same name.
		err := tx.Bucket(f.userId).Bucket(foldersBucket).ForEach(func(k, _ []byte) error {
			if string(k) == name {
				return ErrDuplicateMailbox
			}
			return nil
		})
		if err != nil {
			return err
		}

		// Create the new mailbox.
		b, err := tx.Bucket(f.userId).Bucket(foldersBucket).CreateBucket([]byte(name))
		if err != nil {
			return err
		}

		// Copy everything.
		oldB := tx.Bucket(f.userId).Bucket(foldersBucket).Bucket([]byte(f.name))
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
		return tx.Bucket(f.userId).Bucket(foldersBucket).DeleteBucket([]byte(f.name))
	})
	if err != nil {
		return err
	}

	f.name = name
	return nil
}
