package mem

import (
	"errors"
	
	"github.com/DanielKrawisz/bmagent/store"
)

type message struct {
	payload []byte
	suffix uint64 
}

// Folder that exists in memory rather than in bolt db. 
// Not currently used but possibly useful in the future. 
type memFolder struct {
	name string
	lastIndex uint64
	nextIndex uint64
	lastIndexBySuffix map[uint64]uint64
	messages map[uint64]message
}

func NewFolder(name string) *memFolder {
	return &memFolder {
		name : name, 
		lastIndex : 0, 
		nextIndex : 1,
		messages : make(map[uint64]message), 
		lastIndexBySuffix : make(map[uint64]uint64),
	}
}

func (f *memFolder) Name() string {
	return f.name
}

func (f *memFolder) SetName(name string) error {
	f.name = name
	return nil
}
	
func (f *memFolder) NextID() uint64 {
	return f.nextIndex
}

func (f *memFolder) LastID() (uint64, map[uint64]uint64) {
	return f.lastIndex, f.lastIndexBySuffix
}

func (f *memFolder) updateLast(id, suffix uint64) {
	
	if f.lastIndex < id {
		f.lastIndex = id
		f.lastIndexBySuffix[suffix] = id
		
		return
	}
	
	if f.lastIndexBySuffix[suffix] < id {
		f.lastIndexBySuffix[suffix] = id
	}
}

func (f *memFolder) InsertNewMessage(msg []byte, suffix uint64) (uint64, error) {
	if msg == nil {
		return 0, errors.New("Nil message inserted.")
	}
	
	defer func() {
		f.updateLast(f.nextIndex, suffix)
		f.nextIndex ++
	}()
	
	f.messages[f.nextIndex] = message{payload : msg, suffix : suffix}
	
	return f.nextIndex, nil
}

func (f *memFolder) InsertMessage(id uint64, msg []byte, suffix uint64) error {
	if id == 0 || id >= f.nextIndex {
		return store.ErrInvalidID
	}
	
	if msg == nil {
		return errors.New("Nil message inserted.")
	}
	
	if _, ok := f.messages[id]; ok {
		return store.ErrDuplicateID
	}
	
	f.updateLast(id, suffix)
	
	f.messages[id] = message{payload : msg, suffix : suffix}
	
	return nil
}

func (f *memFolder) GetMessage(id uint64) (uint64, []byte, error) {
	if id == 0 || id >= f.nextIndex {
		return 0, nil, store.ErrNotFound
	}
	
	if m, ok := f.messages[id]; !ok {
		return 0, nil, store.ErrNotFound
	} else {
		return m.suffix, m.payload, nil
	}
}

func (f *memFolder) DeleteMessage(id uint64) error {
	if id == 0 || id >= f.nextIndex {
		return store.ErrInvalidID
	}
	
	var suffix uint64
	if m, ok := f.messages[id]; !ok {
		return store.ErrNotFound
	} else {
		suffix = m.suffix
	}
	
	delete(f.messages, id)
	
	if f.lastIndexBySuffix[suffix] != id {
		return nil
	}
	
	for k := id - 1; k > 0; k -- {
		
		if m, ok := f.messages[k]; ok {
			if id == f.lastIndex {
				f.lastIndex = k
			}
			
			if m.suffix == suffix {
				f.lastIndexBySuffix[suffix] = k;
				return nil
			}
		}
	}
	
	// k has reached zero and the function hasn't returned. 
	delete(f.lastIndexBySuffix, suffix)
	
	if id == f.lastIndex {
		f.lastIndex = 0
	}
	
	return nil
}

func (f *memFolder) ForEachMessage(lowID, highID, suffix uint64,
	fn func(id, suffix uint64, msg []byte) error) error {
	
	for id, m :=  range f.messages {
		if (id > highID && highID != 0) || id < lowID {
			continue	
		}
		
		err := fn(id, m.suffix, m.payload) 
		
		if err != nil {
			return err
		}
	}
	
	return nil
}