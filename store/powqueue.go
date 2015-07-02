// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package store

import (
	"crypto/sha512"
	"encoding/binary"

	"github.com/boltdb/bolt"
)

// PowQueue is a FIFO queue for objects that need proof-of-work done on them.
// It implements Enqueue, Dequeue and Peek; the most basic queue operations.
type PowQueue struct {
	store     *Store
	nextIndex uint64
}

// newPowQueue creates a new PowQueue object from the provided Store.
func newPowQueue(store *Store) (*PowQueue, error) {
	q := &PowQueue{store: store}

	err := store.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(powQueueBucket)
		if err != nil {
			return err
		}
		// Get the index value of the last stored element.
		lastIndex, _ := bucket.Cursor().Last()
		if lastIndex == nil { // No entries
			q.nextIndex = 1
		} else {
			q.nextIndex = binary.BigEndian.Uint64(lastIndex) + 1
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return q, nil
}

// Enqueue adds an object message with a target value for PoW to the end of the
// queue.
func (q *PowQueue) Enqueue(target uint64, objHash *[sha512.Size]byte) error {
	// Key is the next index
	k := make([]byte, 8)
	binary.BigEndian.PutUint64(k, q.nextIndex)

	// Value is stored as: target (8 bytes) || objHash (64 bytes)
	v := make([]byte, 8+sha512.Size)
	binary.BigEndian.PutUint64(v, target)
	copy(v[8:], objHash[:])

	err := q.store.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(powQueueBucket).Put(k, v)
	})
	if err != nil {
		return err
	}

	q.nextIndex++
	return nil
}

// Dequeue removes an object message along with its target PoW value from the
// beginning of the queue.
func (q *PowQueue) Dequeue() (uint64, *[sha512.Size]byte, error) {
	var target uint64
	var objHash [sha512.Size]byte

	err := q.store.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(powQueueBucket)

		k, v := bucket.Cursor().First()
		if k == nil || v == nil { // No elements
			return ErrNotFound
		}

		target = binary.BigEndian.Uint64(v[:8])
		copy(objHash[:], v[8:])

		return bucket.Delete(k)
	})
	if err != nil {
		return 0, nil, err
	}

	return target, &objHash, nil
}

// Peek returns the object that would be removed when Dequeue is run next.
func (q *PowQueue) Peek() (uint64, *[sha512.Size]byte, error) {
	var target uint64
	var objHash [sha512.Size]byte

	err := q.store.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(powQueueBucket)

		k, v := bucket.Cursor().First()
		if k == nil || v == nil { // No elements
			return ErrNotFound
		}

		target = binary.BigEndian.Uint64(v[:8])
		copy(objHash[:], v[8:])
		return nil

	})
	if err != nil {
		return 0, nil, err
	}

	return target, &objHash, nil
}
