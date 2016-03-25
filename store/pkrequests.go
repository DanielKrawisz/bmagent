// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package store

import (
	"encoding/binary"
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
	db    *bolt.DB
}

// initializePKRequestStore initializes the database for pub key requests.
func initializePKRequestStore(db *bolt.DB) error {
	err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(pkRequestsBucket)
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

// New adds a new address to the watchlist along with the current timestamp. If
// the address already exists, it increments the counter value specifying the
// number of times a getpubkey request was sent for the given address and
// returns it.
func (r *PKRequests) New(addr string) (uint32, error) {
	k := []byte(addr)
	var count uint32

	err := r.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(pkRequestsBucket)
		t := bucket.Get(k)
		if t == nil { // Entry doesn't exist, so create it and set counter to 1.
			count = 1
		} else {
			count = binary.BigEndian.Uint32(t[:4]) + 1 // increment
		}

		v := make([]byte, 4)
		binary.BigEndian.PutUint32(v, count)

		t, err := time.Now().MarshalBinary()
		if err != nil {
			return err
		}
		v = append(v, t...)

		return bucket.Put(k, v)
	})
	if err != nil {
		return 0, err
	}

	return count, nil
}

// LastRequestTime returns the time that the last getpubkey request for the
// given address was sent out to the network. If the address doesn't exist, an
// ErrNotFound is returned.
func (r *PKRequests) LastRequestTime(addr string) (time.Time, error) {
	k := []byte(addr)
	var v time.Time

	err := r.db.View(func(tx *bolt.Tx) error {
		t := tx.Bucket(pkRequestsBucket).Get(k)
		if t == nil { // Entry doesn't exist.
			return ErrNotFound
		}
		return v.UnmarshalBinary(t[4:])
	})
	if err != nil {
		return time.Time{}, err
	}

	return v, nil
}

// Remove removes an address from the store.
func (r *PKRequests) Remove(addr string) error {
	k := []byte(addr)

	return r.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(pkRequestsBucket).Delete(k)
	})
}

// ForEach runs the specified function for each item in the public key requests
// store, breaking early if an error occurs.
func (r *PKRequests) ForEach(f func(address string, reqCount uint32,
	lastReqTime time.Time) error) error {

	return r.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(pkRequestsBucket)
		return bucket.ForEach(func(k []byte, v []byte) error {
			address := string(k)
			reqCount := binary.BigEndian.Uint32(v[:4])

			var lastReqTime time.Time
			err := lastReqTime.UnmarshalBinary(v[4:])
			if err != nil {
				return err
			}

			return f(address, reqCount, lastReqTime)
		})
	})
}
