// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package store

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/boltdb/bolt"
)

// Data keys of a mailbox.
var (
	// mailboxTypeKey represents the type of mailbox (MailboxType).
	mailboxTypeKey = []byte("type")

	// mailboxCreatedOnKey contains the time of creation of mailbox.
	mailboxCreatedOnKey = []byte("createdOn")

	// mailboxNewIndex contains the index of the next element. Kept here because
	// IMAP requires existence of unique message IDs that do not change over
	// sessions.
	mailboxNewIndex = []byte("newIndex")

	// mailboxName is a user-friendly name for a mailbox.
	mailboxName = []byte("name")
)

// Mailbox is a mailbox corresponding to a private identity or broadcast. It's
// designed such that any kind of message (not just text based) can be stored
// in the data store.
type Mailbox struct {
	store   *Store
	address string // Address corresponding to the mailbox.
	boxType MailboxType
	name    string
}

// newMailbox creates a new Mailbox, initializing the database if necessary.
func newMailbox(store *Store, address string,
	boxType MailboxType) (*Mailbox, error) {

	mbox := &Mailbox{
		store:   store,
		address: address,
		boxType: boxType,
	}

	err := store.db.Update(func(tx *bolt.Tx) error {
		// Get bucket for mailbox.
		bucket := tx.Bucket(mailboxesBucket).Bucket([]byte(address))

		if bucket == nil {
			// New mailbox so initialize it with all the data.
			bucket, err := tx.Bucket(mailboxesBucket).CreateBucket([]byte(address))
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

			one := make([]byte, 8)
			binary.BigEndian.PutUint64(one, 1)

			err = data.Put(mailboxTypeKey, []byte{byte(boxType)})
			if err != nil {
				return err
			}

			err = data.Put(mailboxCreatedOnKey, now)
			if err != nil {
				return err
			}

			err = data.Put(mailboxNewIndex, one)
			if err != nil {
				return err
			}

			err = data.Put(mailboxName, []byte("")) // empty name by default
			if err != nil {
				return nil
			}
		} else {
			// Override the user provided boxType with the actual value.
			mbox.boxType = MailboxType(bucket.Bucket(mailboxDataBucket).
				Get(mailboxTypeKey)[0])
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("Failed to create/load mailbox with address %s: %v",
			address, err)
	}

	return mbox, nil
}

// InsertMessage inserts a new message with the specified suffix into the
// mailbox and returns its index value. For normal mailboxes, suffix could be
// the encoding type. For special use mailboxes like "Pending", suffix could be
// used as a 'key', like a reason code (why the message is marked as Pending).
func (mbox *Mailbox) InsertMessage(msg []byte, suffix uint64) (uint64, error) {
	var index uint64

	enc, err := mbox.store.encrypt(msg)
	if err != nil {
		return 0, err
	}

	err = mbox.store.db.Update(func(tx *bolt.Tx) error {
		m := tx.Bucket(mailboxesBucket).Bucket([]byte(mbox.address))
		data := m.Bucket(mailboxDataBucket)

		idxBytes := data.Get(mailboxNewIndex)
		index = binary.BigEndian.Uint64(idxBytes)

		k := make([]byte, 16)
		copy(k[:8], idxBytes)                     // first half
		binary.BigEndian.PutUint64(k[8:], suffix) // second half

		// Insert message using newIndex as the first 8 bytes and suffix as the
		// latter 8 bytes of the key.
		err := m.Put(k, enc)
		if err != nil {
			return err
		}

		// Increment newIndex.
		newIndex := make([]byte, 8)
		binary.BigEndian.PutUint64(newIndex, index+1)
		err = data.Put(mailboxNewIndex, newIndex)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return 0, err
	}

	return index, nil
}

// GetMessage retrieves a message from the mailbox by its index. It returns the
// suffix and the message. An error is returned if the message with the given
// index doesn't exist in the database.
func (mbox *Mailbox) GetMessage(index uint64) (uint64, []byte, error) {
	var suffix uint64
	var msg []byte

	var success bool

	idxBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(idxBytes, index)

	err := mbox.store.db.View(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(mailboxesBucket).Bucket([]byte(mbox.address)).Cursor()
		k, v := cursor.Seek(idxBytes)

		// Check if the first 8 bytes match the index.
		if k == nil || v == nil || !bytes.Equal(k[:8], idxBytes) {
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

// ForEachMessage runs the given function for each message in the store,
// terminating early if an error occurs. It iterates in the increasing order of
// index.
//
// Make sure the function doesn't take long to execute. DO NOT execute any other
// database operations in it.
func (mbox *Mailbox) ForEachMessage(f func(index uint64, suffix uint64,
	msg []byte) error) error {

	return mbox.store.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(mailboxesBucket).Bucket([]byte(mbox.address)).
			ForEach(func(k, v []byte) error {

			// We need this safeguard because BoltDB also loops over buckets. We
			// don't want to include the bucket "data" in our query.
			if len(k) < 16 {
				return nil
			}

			index := binary.BigEndian.Uint64(k[:8])
			suffix := binary.BigEndian.Uint64(k[8:])

			msg, success := mbox.store.decrypt(v)
			if !success {
				return ErrDecryptionFailed
			}

			return f(index, suffix, msg)
		})
	})
}

// ForEachMessageBySuffix runs the given function for each message which has
// suffix as its key suffix. It's useful for getting all messages of a
// particular type. For example, retrieving all messages with encoding type 2.
// It iterates in the increasing order of index.
//
// Make sure the function doesn't take long to execute. DO NOT execute any other
// database operations in it.
func (mbox *Mailbox) ForEachMessageBySuffix(suffix uint64, f func(index uint64,
	msg []byte) error) error {

	sfxBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(sfxBytes, suffix)

	return mbox.store.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(mailboxesBucket).Bucket([]byte(mbox.address)).
			ForEach(func(k, v []byte) error {

			// We need this safeguard because BoltDB also loops over buckets. We
			// don't want to include the bucket "data" in our query.
			if len(k) < 16 {
				return nil
			}

			index := binary.BigEndian.Uint64(k[:8])
			if bytes.Equal(k[8:], sfxBytes) { // Suffix matches what we expect.
				msg, success := mbox.store.decrypt(v)
				if !success {
					return ErrDecryptionFailed
				}

				return f(index, msg)
			}
			return nil
		})
	})
}

// DeleteMessage deletes a message with the given index from the store. An error
// is returned if the message doesn't exist in the store.
func (mbox *Mailbox) DeleteMessage(index uint64) error {
	idxBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(idxBytes, index)

	return mbox.store.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(mailboxesBucket).Bucket([]byte(mbox.address))
		k, v := bucket.Cursor().Seek(idxBytes)

		// Check if the first 8 bytes match the index.
		if k == nil || v == nil || !bytes.Equal(k[:8], idxBytes) {
			return ErrNotFound
		}

		return bucket.Delete(k)
	})
}

// GetName returns the user-friendly name of the mailbox.
func (mbox *Mailbox) GetName() string {
	return mbox.name
}

// SetName sets a user-friendly name for the mailbox. Mailboxes of the same
// type cannot have a matching name.
func (mbox *Mailbox) SetName(name string) error {
	err := mbox.store.db.Update(func(tx *bolt.Tx) error {
		// Check if some other mailbox of the same type has the same name.
		err := tx.Bucket(mailboxesBucket).ForEach(func(addr, _ []byte) error {
			b := tx.Bucket(mailboxesBucket).Bucket(addr).Bucket(mailboxDataBucket)
			if MailboxType(b.Get(mailboxTypeKey)[0]) == mbox.boxType &&
				string(b.Get(mailboxName)) == name {
				return ErrDuplicateName
			}
			return nil
		})
		if err != nil {
			return err
		}

		return tx.Bucket(mailboxesBucket).Bucket([]byte(mbox.address)).
			Bucket(mailboxDataBucket).Put(mailboxName, []byte(name))
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
		err := tx.Bucket(mailboxesBucket).DeleteBucket([]byte(mbox.address))
		if err != nil {
			return err
		}

		// Delete mailbox from store.
		delete(mbox.store.mailboxes, mbox.address)
		return nil
	})
}

// GetType returns the type of the mailbox.
func (mbox *Mailbox) GetType() MailboxType {
	return mbox.boxType
}
