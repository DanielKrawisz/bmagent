// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package message

import (
	"errors"
	"fmt"
)

// membox is a Mailbox that is stored in memory, mainly for testing purposes.
type membox struct {
	name    string
	nextUID uint64
	bm      map[uint64]struct {
		message []byte
		suffix  uint64
	}
}

// GetNextID returns the next id that will be assigned.
func (box *membox) GetNextID() (uint64, error) {
	return box.nextUID, nil
}

// Name gives name of the mailbox
// It is part of the BitmessageFolder interface.
func (box *membox) GetName() string {
	return box.name
}

func (box *membox) GetMessage(id uint64) (uint64, []byte, error) {
	entry, ok := box.bm[id]
	if !ok {
		return 0, nil, errors.New("No such id")
	}

	return entry.suffix, entry.message, nil
}

func (box *membox) InsertMessage(msg []byte, id, suffix uint64) (uint64, error) {
	if msg == nil {
		panic("why are we inserting a nil message?")
		return 0, errors.New("Nil msg")
	}
	if id != 0 {
		if _, ok := box.bm[id]; ok {
			return id, errors.New("Id already taken")
		}
	}

	id = box.nextUID
	box.nextUID++

	box.bm[id] = struct {
		message []byte
		suffix  uint64
	}{
		message: msg,
		suffix:  suffix,
	}

	return id, nil
}

func (box *membox) DeleteMessage(id uint64) error {
	if _, ok := box.bm[id]; !ok {
		return errors.New("No such id")
	}

	delete(box.bm, id)
	return nil
}

// LastUID returns the UID of the very last message in the mailbox
func (box *membox) GetLastID() (uint64, error) {
	var maxID uint64
	if len(box.bm) == 0 {
		return 0, errors.New("Mailbox empty.")
	}

	for id, _ := range box.bm {
		if id > maxID {
			maxID = id
		}
	}

	return maxID, nil
}

// GetLastIDBySuffix returns the UID of the last message in the mailbox
// with the given suffix.
func (box *membox) GetLastIDBySuffix(suffix uint64) (uint64, error) {
	var maxID uint64
	if len(box.bm) == 0 {
		return 0, errors.New("Mailbox empty.")
	}

	for id, entry := range box.bm {
		if id > maxID && entry.suffix == suffix {
			maxID = id
		}
	}

	if maxID == 0 {
		return 0, errors.New("None found.")
	}
	return maxID, nil
}

func (box *membox) ForEachMessage(lowID, highID, suffix uint64,
	f func(id, suffix uint64, msg []byte) error) error {
	fmt.Println("for each message:", lowID, highID, suffix)
	var err error
	for id, entry := range box.bm {
		fmt.Println("for each msg loop: ", id, ", ", entry.suffix)
		if id >= lowID && (id <= highID || highID == 0) && (suffix == 0 || suffix == entry.suffix) {
			err = f(id, entry.suffix, entry.message)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// NewMembox returns a new membox.
func NewMembox(name string) Mailbox {
	return &membox{
		name:    name,
		nextUID: 1,
		bm: make(map[uint64]struct {
			message []byte
			suffix  uint64
		}),
	}
}
