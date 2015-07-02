// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package store

import (
	"time"

	"github.com/boltdb/bolt"
)

// PKRequests is a store that keeps track of getpubkey requests sent to the
// Bitmessage network. It stores the requested address along with the time
// the request was made. All addresses passed are expected to be valid
// Bitmessage addresses. A separate goroutine routinely queries the bmd RPC
// server for the public keys corresponding to the addresses in this store and
// removes them if bmd has received them. Thus, PKRequests serves as a
// watchlist.
type PKRequests struct {
	store *Store
}

// newPKRequestStore creates a new PKRequests object after doing the necessary
// initialization.
func newPKRequestStore(store *Store) (*PKRequests, error) {
	r := &PKRequests{store: store}

	err := store.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(pkRequestsBucket)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return r, nil
}

// New adds a new address to the watchlist along with the current timestamp.
func (r *PKRequests) New(addr string) error {
	k := []byte(addr)
	v, err := time.Now().MarshalBinary()
	if err != nil {
		return err
	}

	return r.store.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(pkRequestsBucket).Put(k, v)
	})
}

// GetTime returns the time that an address was added to the watchlist. If the
// address doesn't exist, an ErrNotFound is returned.
func (r *PKRequests) GetTime(addr string) (time.Time, error) {
	k := []byte(addr)
	var v time.Time

	err := r.store.db.View(func(tx *bolt.Tx) error {
		t := tx.Bucket(pkRequestsBucket).Get(k)
		if t == nil { // Entry doesn't exist.
			return ErrNotFound
		}
		return v.UnmarshalBinary(t)
	})
	if err != nil {
		return time.Time{}, err
	}

	return v, nil
}

// Remove removes an address from the store.
func (r *PKRequests) Remove(addr string) error {
	k := []byte(addr)

	return r.store.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(pkRequestsBucket).Delete(k)
	})
}

// ForEach runs the specified function for each item in the public key requests
// store in a separate goroutine, breaking early if an error occurs.
func (r *PKRequests) ForEach(f func(address string, addTime time.Time)) error {
	return r.store.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(pkRequestsBucket)
		return bucket.ForEach(func(k []byte, v []byte) error {
			address := string(k)
			var addTime time.Time
			err := addTime.UnmarshalBinary(v)
			if err != nil {
				return err
			}

			go f(address, addTime)
			return nil
		})
	})
}
