// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package store

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/boltdb/bolt"
)

// Data keys of a mailbox.
var (
	// mailboxCreatedOnKey contains the time of creation of mailbox.
	mailboxCreatedOnKey = []byte("createdOn")

	// ErrDuplicateID is returned by InsertMessage when the a message with the
	// specified ID already exists in the mailbox.
	ErrDuplicateID = errors.New("duplicate ID")
)

// Mailbox is a mailbox corresponding to a private identity or broadcast. It's
// designed such that any kind of message (not just text based) can be stored
// in the data store.
type Mailbox struct {
	store *Store
	name  string
}

// NewMailbox creates a new Mailbox, initializing the database if specified.
func NewMailbox(store *Store, name string, verifyOrCreate bool) (*Mailbox, error) {
	mbox := &Mailbox{
		store: store,
		name:  name,
	}
	if !verifyOrCreate {
		return mbox, nil
	}

	err := store.db.Update(func(tx *bolt.Tx) error {
		// Get bucket for mailbox.
		bucket := tx.Bucket(mailboxesBucket).Bucket([]byte(name))

		if bucket == nil {
			// New mailbox so initialize it with all the data.
			bucket, err := tx.Bucket(mailboxesBucket).CreateBucket([]byte(name))
			if err != nil {
				return err
			}

			data, err := bucket.CreateBucket(mailboxDataBucket)
			if err != nil {
				return err
			}

			now, err := time.Now().MarshalBinary()
			if err != nil {
				return err
			}

			err = data.Put(mailboxCreatedOnKey, now)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("Failed to create/load mailbox %s: %v",
			name, err)
	}

	return mbox, nil
}

// InsertMessage inserts a new message with the specified suffix and id into the
// mailbox and returns the ID. If input id is 0, then the store automatically
// generates a unique index value. For normal mailboxes, suffix could be the
// encoding type. For special use mailboxes like "Pending", suffix could be
// used as a 'key', like a reason code (why the message is marked as Pending).
func (mbox *Mailbox) InsertMessage(msg []byte, id, suffix uint64) (uint64, error) {
	enc, err := mbox.store.encrypt(msg)
	if err != nil {
		return 0, err
	}

	err = mbox.store.db.Update(func(tx *bolt.Tx) error {
		m := tx.Bucket(mailboxesBucket).Bucket([]byte(mbox.name))

		// Generate a new ID if we asked for it.
		if id == 0 {
			id = binary.BigEndian.Uint64(tx.Bucket(miscBucket).Get(mailboxLatestIDKey)) + 1

			// Increment mailboxLatestID.
			idB := make([]byte, 8)
			binary.BigEndian.PutUint64(idB, id)

			err = tx.Bucket(miscBucket).Put(mailboxLatestIDKey, idB)
			if err != nil {
				return err
			}
		}

		// Insert message using ID as the first 8 bytes and suffix as the
		// latter 8 bytes of the key.
		k := make([]byte, 16)
		binary.BigEndian.PutUint64(k[:8], id)     // first half
		binary.BigEndian.PutUint64(k[8:], suffix) // second half

		// Check if a message with the given ID already exists.
		kk, _ := m.Cursor().Seek(k[:8])
		if kk != nil && bytes.Equal(kk[:8], k[:8]) {
			return ErrDuplicateID
		}

		return m.Put(k, enc)
	})
	if err != nil {
		return 0, err
	}

	return id, nil
}

// GetMessage retrieves a message from the mailbox by its index. It returns the
// suffix and the message. An error is returned if the message with the given
// index doesn't exist in the database.
func (mbox *Mailbox) GetMessage(id uint64) (uint64, []byte, error) {
	var suffix uint64
	var msg []byte

	var success bool

	idBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(idBytes, id)

	err := mbox.store.db.View(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(mailboxesBucket).Bucket([]byte(mbox.name)).Cursor()
		k, v := cursor.Seek(idBytes)

		// Check if the first 8 bytes match the index.
		if k == nil || v == nil || !bytes.Equal(k[:8], idBytes) {
			return ErrNotFound
		}

		suffix = binary.BigEndian.Uint64(k[8:])
		msg, success = mbox.store.decrypt(v)
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
func (mbox *Mailbox) ForEachMessage(lowID, highID, suffix uint64,
	f func(id, suffix uint64, msg []byte) error) error {
	if highID != 0 && lowID > highID {
		return errors.New("Nice try, son. But lowID cannot be greater than highID.")
	}

	bLowID := make([]byte, 8)
	binary.BigEndian.PutUint64(bLowID, lowID)

	return mbox.store.db.View(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(mailboxesBucket).Bucket([]byte(mbox.name)).Cursor()

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

			msg, success := mbox.store.decrypt(v)
			if !success {
				fmt.Println("About to return a decryption failed error.")
				return ErrDecryptionFailed
			}

			if err := f(id, sfx, msg); err != nil {
				return err
			}
		}

		return nil
	})
}

// DeleteMessage deletes a message with the given index from the store. An error
// is returned if the message doesn't exist in the store.
func (mbox *Mailbox) DeleteMessage(id uint64) error {
	idxBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(idxBytes, id)

	return mbox.store.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(mailboxesBucket).Bucket([]byte(mbox.name))
		k, v := bucket.Cursor().Seek(idxBytes)

		// Check if the first 8 bytes match the index.
		if k == nil || v == nil || !bytes.Equal(k[:8], idxBytes) {
			return ErrNotFound
		}

		return bucket.Delete(k)
	})
}

// Name returns the user-friendly name of the mailbox.
func (mbox *Mailbox) Name() string {
	return mbox.name
}

// SetName changes the name of the mailbox.
func (mbox *Mailbox) SetName(name string) error {
	if name == "" {
		return errors.New("Name cannot be empty.")
	}

	err := mbox.store.db.Update(func(tx *bolt.Tx) error {
		// Check if some other mailbox has the same name.
		err := tx.Bucket(mailboxesBucket).ForEach(func(k, _ []byte) error {
			if string(k) == name {
				return ErrDuplicateMailbox
			}
			return nil
		})
		if err != nil {
			return err
		}

		// Create the new mailbox.
		b, err := tx.Bucket(mailboxesBucket).CreateBucket([]byte(name))
		if err != nil {
			return err
		}

		// Copy everything.
		oldB := tx.Bucket(mailboxesBucket).Bucket([]byte(mbox.name))
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
		return tx.Bucket(mailboxesBucket).DeleteBucket([]byte(mbox.name))
	})
	if err != nil {
		return err
	}

	mbox.name = name
	return nil
}

// Delete deletes the mailbox. Any operations after a Delete are invalid.
func (mbox *Mailbox) Delete() error {
	return mbox.store.db.Update(func(tx *bolt.Tx) error {
		// Delete the mailbox and associated data.
		err := tx.Bucket(mailboxesBucket).DeleteBucket([]byte(mbox.name))
		if err != nil {
			return err
		}

		// Delete mailbox from store.
		delete(mbox.store.mailboxes, mbox.name)
		return nil
	})
}

// NextID returns the next index value that will be assigned in the mailbox..
func (mbox *Mailbox) NextID() (uint64, error) {
	var id uint64

	err := mbox.store.db.View(func(tx *bolt.Tx) error {
		id = binary.BigEndian.Uint64(tx.Bucket(miscBucket).Get(mailboxLatestIDKey)) + 1
		return nil
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

// LastID returns the highest index value in the mailbox.
func (mbox *Mailbox) LastID() (uint64, error) {
	var id uint64

	err := mbox.store.db.View(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(mailboxesBucket).Bucket([]byte(mbox.name)).Cursor()

		// Read from the end until we have a non-nil value.
		for k, _ := cursor.Last(); ; k, _ = cursor.Prev() {
			if k == nil { // No records
				return ErrNotFound
			}
			if len(k) != 16 { // Don't want to read key from a bucket.
				continue
			}
			id = binary.BigEndian.Uint64(k[:8])
			return nil
		}
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

// LastIDBySuffix returns the highest index value from messages with the
// specified suffix in the mailbox.
func (mbox *Mailbox) LastIDBySuffix(suffix uint64) (uint64, error) {
	var id uint64
	err := mbox.store.db.View(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(mailboxesBucket).Bucket([]byte(mbox.name)).Cursor()

		// Loop from the end, returning the first found match.
		for k, _ := cursor.Last(); k != nil; k, _ = cursor.Prev() {
			if len(k) != 16 { // Don't want to loop over buckets.
				continue
			}
			sfx := binary.BigEndian.Uint64(k[8:])
			if sfx == suffix {
				id = binary.BigEndian.Uint64(k[:8])
				return nil
			}
		}
		return ErrNotFound
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}
