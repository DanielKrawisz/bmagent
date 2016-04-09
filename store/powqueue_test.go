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
	uname := "daniel"
	l, err := store.Open(fName)
	s, q, _, err := l.Construct(uname, pass)
	if err != nil {
		t.Fatal(err)
	}

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
	_, _, _, err = q.Dequeue()
	if err != store.ErrNotFound {
		t.Errorf("Dequeue didn't give expected error %v, got %v",
			store.ErrNotFound, err)
	}

	// First enqueue.
	var u uint32 = 12
	idx, err := q.Enqueue(target, u, obj)
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
	_, q, _, err = l.Construct(uname, pass)
	if err != nil {
		t.Fatal(err)
	}

	// Second enqueue.
	var u1 uint32 = 37
	idx1, err := q.Enqueue(target1, u1, obj1)
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
	idxT, uT, objT, err := q.Dequeue()
	if err != nil {
		t.Error("Dequeue failed:", err)
	}
	if !bytes.Equal(obj, objT) {
		t.Errorf("Expected %v got %v", obj, objT)
	}
	if idx != idxT {
		t.Errorf("Expected %d got %d", idx, idxT)
	}
	if u != uT {
		t.Errorf("Expected %d got %d", u, uT)
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
	idxT, uT, objT, err = q.Dequeue()
	if err != nil {
		t.Error("Dequeue failed:", err)
	}
	if !bytes.Equal(obj1, objT) {
		t.Errorf("Expected %v got %v", obj1, objT)
	}
	if idx1 != idxT {
		t.Errorf("Expected %d got %d", idx1, idxT)
	}
	if u1 != uT {
		t.Errorf("Expected %d got %d", u1, uT)
	}

	// PeekForPow should fail.
	_, _, err = q.PeekForPow()
	if err != store.ErrNotFound {
		t.Errorf("PeekForPow didn't give expected error %v, got %v",
			store.ErrNotFound, err)
	}

	// Dequeue should fail.
	_, _, _, err = q.Dequeue()
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
