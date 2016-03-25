// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package store

import (
	"bytes"

	"github.com/boltdb/bolt"
	"github.com/DanielKrawisz/bmutil"
)

// BroadcastAddresses keeps track of the broadcasts that the user is listening
// to. It provides functionality for adding, removal and running a function for
// each address.
type BroadcastAddresses struct {
	db *bolt.DB
	addrs []bmutil.Address // All broadcast addresses.
}

// newBroadcastsStore creates a new BroadcastAddresses object after doing the
// necessary initialization.
func newBroadcastsStore(db *bolt.DB) (*BroadcastAddresses, error) {
	b := &BroadcastAddresses{
		db : db,
		addrs: make([]bmutil.Address, 0),
	}

	err := db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(broadcastAddressesBucket)
		if err != nil {
			return err
		}

		return bucket.ForEach(func(k, _ []byte) error {
			addr, err := bmutil.DecodeAddress(string(k))
			if err != nil {
				return err
			}

			b.addrs = append(b.addrs, *addr)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	return b, nil
}

// Add adds a new address to the store.
func (b *BroadcastAddresses) Add(address string) error {
	addr, err := bmutil.DecodeAddress(address)
	if err != nil {
		return err
	}

	k := []byte(address)
	v := []byte{}

	err = b.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(broadcastAddressesBucket).Put(k, v)
	})
	if err != nil {
		return err
	}

	b.addrs = append(b.addrs, *addr)
	return nil
}

// Remove removes an address from the store.
func (b *BroadcastAddresses) Remove(address string) error {
	addr, err := bmutil.DecodeAddress(address)
	if err != nil {
		return err
	}

	k := []byte(address)

	err = b.db.Update(func(tx *bolt.Tx) error {
		if tx.Bucket(broadcastAddressesBucket).Get(k) == nil {
			return ErrNotFound
		}

		return tx.Bucket(broadcastAddressesBucket).Delete(k)
	})
	if err != nil {
		return err
	}

	for i, t := range b.addrs {
		if bytes.Equal(t.Ripe[:], addr.Ripe[:]) {
			// Delete.
			b.addrs = append(b.addrs[:i], b.addrs[i+1:]...)
			return nil
		}
	}
	return nil
}

// ForEach runs the specified function for each broadcast address, breaking
// early if an error occurs.
func (b *BroadcastAddresses) ForEach(f func(address *bmutil.Address) error) error {
	for _, addr := range b.addrs {
		err := f(&addr)
		if err != nil {
			return err
		}
	}
	return nil
}
