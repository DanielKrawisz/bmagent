// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package store_test

import (
	"bytes"
	"crypto/sha512"
	"io/ioutil"
	"os"
	"testing"

	"github.com/monetas/bmclient/store"
	"github.com/monetas/bmutil"
)

func TestPowQueue(t *testing.T) {
	// Open store.
	f, err := ioutil.TempFile("", "tempstore")
	if err != nil {
		t.Fatal(err)
	}
	fName := f.Name()
	f.Close()

	pass := []byte("password")
	s, err := store.Open(fName, pass)
	if err != nil {
		t.Fatal(err)
	}

	// Start.
	q := s.PowQueue

	var hash [sha512.Size]byte
	copy(hash[:], bmutil.Sha512([]byte("test")))
	target := uint64(123)

	var hash1 [sha512.Size]byte
	copy(hash1[:], bmutil.Sha512([]byte("test1")))
	target1 := uint64(456)

	// Peek should fail.
	_, _, err = q.Peek()
	if err != store.ErrNotFound {
		t.Errorf("Peek didn't give expected error %v, got %v",
			store.ErrNotFound, err)
	}

	// Dequeue should fail.
	_, _, err = q.Dequeue()
	if err != store.ErrNotFound {
		t.Errorf("Dequeue didn't give expected error %v, got %v",
			store.ErrNotFound, err)
	}

	// First enqueue.
	err = q.Enqueue(target, &hash)
	if err != nil {
		t.Error("Enqueue failed:", err)
	}

	// First peek.
	targetT, hashT, err := q.Peek()
	if err != nil {
		t.Error("Peek failed:", err)
	}
	if target != targetT {
		t.Errorf("Expected %d got %d", target, targetT)
	}
	if !bytes.Equal(hash[:], hashT[:]) {
		t.Errorf("Expected %v got %v", hash, hashT)
	}

	// Close and re-open database to test if ordering is still preserved.
	err = s.Close()
	if err != nil {
		t.Fatal(err)
	}
	s, err = store.Open(fName, pass)
	if err != nil {
		t.Fatal(err)
	}
	q = s.PowQueue

	// Second enqueue.
	err = q.Enqueue(target1, &hash1)
	if err != nil {
		t.Error("Enqueue failed:", err)
	}

	// Peek again, should still give same answers.
	targetT, hashT, err = q.Peek()
	if err != nil {
		t.Error("Peek failed:", err)
	}
	if target != targetT {
		t.Errorf("Expected %d got %d", target, targetT)
	}
	if !bytes.Equal(hash[:], hashT[:]) {
		t.Errorf("Expected %v got %v", hash, hashT)
	}

	// First dequeue.
	targetT, hashT, err = q.Dequeue()
	if err != nil {
		t.Error("Dequeue failed:", err)
	}
	if target != targetT {
		t.Errorf("Expected %d got %d", target, targetT)
	}
	if !bytes.Equal(hash[:], hashT[:]) {
		t.Errorf("Expected %v got %v", hash, hashT)
	}

	// Second peek. Second item should move to first now.
	targetT, hashT, err = q.Peek()
	if err != nil {
		t.Error("Peek failed:", err)
	}
	if target1 != targetT {
		t.Errorf("Expected %d got %d", target1, targetT)
	}
	if !bytes.Equal(hash1[:], hashT[:]) {
		t.Errorf("Expected %v got %v", hash1, hashT)
	}

	// Second dequeue.
	targetT, hashT, err = q.Dequeue()
	if err != nil {
		t.Error("Dequeue failed:", err)
	}
	if target1 != targetT {
		t.Errorf("Expected %d got %d", target1, targetT)
	}
	if !bytes.Equal(hash1[:], hashT[:]) {
		t.Errorf("Expected %v got %v", hash1, hashT)
	}

	// Peek should fail.
	_, _, err = q.Peek()
	if err != store.ErrNotFound {
		t.Errorf("Peek didn't give expected error %v, got %v",
			store.ErrNotFound, err)
	}

	// Dequeue should fail.
	_, _, err = q.Dequeue()
	if err != store.ErrNotFound {
		t.Errorf("Dequeue didn't give expected error %v, got %v",
			store.ErrNotFound, err)
	}

	// Close database.
	err = s.Close()
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(fName)
}
