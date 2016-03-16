// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package store

import (
	"encoding/binary"

	"github.com/boltdb/bolt"
	"github.com/DanielKrawisz/bmutil"
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
		_, err := tx.CreateBucketIfNotExists(powQueueBucket)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return q, nil
}

// Enqueue adds an object message with a target value for PoW to the end of the
// queue. It returns the index value of the stored element.
func (q *PowQueue) Enqueue(target uint64, obj []byte) (uint64, error) {
	var idx uint64

	// Value is stored as: target (8 bytes) || object
	v := make([]byte, 8+len(obj))
	binary.BigEndian.PutUint64(v, target)
	copy(v[8:], obj)

	err := q.store.db.Update(func(tx *bolt.Tx) error {
		idx = binary.BigEndian.Uint64(tx.Bucket(miscBucket).Get(powQueueLatestIDKey)) + 1

		// Increment powQueueLatestID. That'll be our k.
		k := make([]byte, 8)
		binary.BigEndian.PutUint64(k, idx)

		err := tx.Bucket(miscBucket).Put(powQueueLatestIDKey, k)
		if err != nil {
			return err
		}
		return tx.Bucket(powQueueBucket).Put(k, v)
	})
	if err != nil {
		return 0, err
	}

	return idx, nil
}

// Dequeue removes the object message at beginning of the queue and returns its
// index and itself.
func (q *PowQueue) Dequeue() (uint64, []byte, error) {
	var idx uint64
	var obj []byte
	err := q.store.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(powQueueBucket)

		k, v := bucket.Cursor().First()
		if k == nil || v == nil { // No elements
			return ErrNotFound
		}
		idx = binary.BigEndian.Uint64(k)
		obj = make([]byte, len(v[8:]))
		copy(obj, v[8:])

		return bucket.Delete(k)
	})
	if err != nil {
		return 0, nil, err
	}

	return idx, obj, nil
}

// PeekForPow returns the target and hash values for the object that would be
// removed when Dequeue is run next.
func (q *PowQueue) PeekForPow() (uint64, []byte, error) {
	var target uint64
	var hash []byte

	err := q.store.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(powQueueBucket)

		k, v := bucket.Cursor().First()
		if k == nil || v == nil { // No elements
			return ErrNotFound
		}

		target = binary.BigEndian.Uint64(v[:8])
		hash = bmutil.Sha512(v[8:])
		return nil

	})
	if err != nil {
		return 0, nil, err
	}

	return target, hash, nil
}
