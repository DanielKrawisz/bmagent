// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package store_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/DanielKrawisz/bmagent/store"
	"github.com/DanielKrawisz/bmutil"
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
	l, err := store.Open(fName)
	s, err := l.Construct(pass)
	if err != nil {
		t.Fatal(err)
	}

	// Start.
	q := s.PowQueue

	obj := []byte("test")
	hash := bmutil.Sha512(obj)
	target := uint64(123)

	obj1 := []byte("test1")
	hash1 := bmutil.Sha512(obj1)
	target1 := uint64(456)

	// PeekForPow should fail.
	_, _, err = q.PeekForPow()
	if err != store.ErrNotFound {
		t.Errorf("PeekForPow didn't give expected error %v, got %v",
			store.ErrNotFound, err)
	}

	// Dequeue should fail.
	_, _, err = q.Dequeue()
	if err != store.ErrNotFound {
		t.Errorf("Dequeue didn't give expected error %v, got %v",
			store.ErrNotFound, err)
	}

	// First enqueue.
	idx, err := q.Enqueue(target, obj)
	if err != nil {
		t.Error("Enqueue failed:", err)
	}
	if idx != 1 {
		t.Errorf("Expected %d got %d", 1, idx)
	}

	// First PeekForPow.
	targetT, hashT, err := q.PeekForPow()
	if err != nil {
		t.Error("PeekForPow failed:", err)
	}
	if target != targetT {
		t.Errorf("Expected %d got %d", target, targetT)
	}
	if !bytes.Equal(hash, hashT) {
		t.Errorf("Expected %v got %v", hash, hashT)
	}

	// Close and re-open database to test if ordering is still preserved.
	err = s.Close()
	if err != nil {
		t.Fatal(err)
	}
	l, err = store.Open(fName)
	s, err = l.Construct(pass)
	if err != nil {
		t.Fatal(err)
	}
	q = s.PowQueue

	// Second enqueue.
	idx1, err := q.Enqueue(target1, obj1)
	if err != nil {
		t.Error("Enqueue failed:", err)
	}
	if idx1 != 2 {
		t.Errorf("Expected %d got %d", 2, idx1)
	}

	// PeekForPow again, should still give same answers.
	targetT, hashT, err = q.PeekForPow()
	if err != nil {
		t.Error("PeekForPow failed:", err)
	}
	if target != targetT {
		t.Errorf("Expected %d got %d", target, targetT)
	}
	if !bytes.Equal(hash, hashT) {
		t.Errorf("Expected %v got %v", hash, hashT)
	}

	// First dequeue.
	idxT, objT, err := q.Dequeue()
	if err != nil {
		t.Error("Dequeue failed:", err)
	}
	if !bytes.Equal(obj, objT) {
		t.Errorf("Expected %v got %v", obj, objT)
	}
	if idx != idxT {
		t.Errorf("Expected %d got %d", idx, idxT)
	}

	// Second PeekForPow. Second item should move to first now.
	targetT, hashT, err = q.PeekForPow()
	if err != nil {
		t.Error("PeekForPow failed:", err)
	}
	if target1 != targetT {
		t.Errorf("Expected %d got %d", target1, targetT)
	}
	if !bytes.Equal(hash1, hashT) {
		t.Errorf("Expected %v got %v", hash1, hashT)
	}

	// Second dequeue.
	idxT, objT, err = q.Dequeue()
	if err != nil {
		t.Error("Dequeue failed:", err)
	}
	if !bytes.Equal(obj1, objT) {
		t.Errorf("Expected %v got %v", obj1, objT)
	}
	if idx1 != idxT {
		t.Errorf("Expected %d got %d", idx, idxT)
	}

	// PeekForPow should fail.
	_, _, err = q.PeekForPow()
	if err != store.ErrNotFound {
		t.Errorf("PeekForPow didn't give expected error %v, got %v",
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
