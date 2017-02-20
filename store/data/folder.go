package data

import (
	"errors"
)

var (
	// ErrDuplicateID is returned by InsertMessage when the a message with the
	// specified ID already exists in the folder.
	ErrDuplicateID = errors.New("duplicate ID")

	// ErrInvalidID is returned when an invalid id is provided.
	ErrInvalidID = errors.New("invalid ID")

	// ErrNotFound is returned when a record matching the query or no record at
	// all is found in the database.
	ErrNotFound = errors.New("record not found")
)

// Folder represents a store of data indexed by an id number.
type Folder interface {
	// InsertNewMessage inserts a new message with the specified suffix and
	// id into the folder and returns the ID. For normal folders, suffix
	// could be the encoding type. For special use folders like "Pending",
	// suffix could be used as a 'key', like a reason code (why the message is
	// marked as Pending).
	InsertNewMessage(msg []byte, suffix uint64) (uint64, error)

	// InsertMessage inserts a new message with the specified suffix and id
	// into the folder and returns the ID. If input id is 0 or >= NextID,
	// ErrInvalidID is returned.
	InsertMessage(id uint64, msg []byte, suffix uint64) error

	// GetMessage retrieves a message from the folder by its index. It returns the
	// suffix and the message. An error is returned if the message with the given
	// index doesn't exist in the database.
	GetMessage(id uint64) (uint64, []byte, error)

	// DeleteMessage deletes a message with the given index from the store. An error
	// is returned if the message doesn't exist in the store.
	DeleteMessage(id uint64) error

	// ForEachMessage runs the given function for messages that have IDs between
	// lowID and highID with the given suffix. If lowID is 0, it starts from the
	// first message. If highID is 0, it returns all messages with id >= lowID with
	// the given suffix. If suffix is zero, it returns all messages between lowID
	// and highID, irrespective of the suffix. Note that any combination of lowID,
	// highID and suffix can be zero for the desired results. Both lowID and highID
	// are inclusive.
	//
	// Suffix is useful for getting all messages of a particular type. For example,
	// retrieving all messages with encoding type 2.
	//
	// The function terminates early if an error occurs and iterates in the
	// increasing order of index. Make sure it doesn't take long to execute. DO NOT
	// execute any other database operations in it.
	ForEachMessage(lowID, highID, suffix uint64,
		fn func(id, suffix uint64, msg []byte) error) error

	// NextID returns the next index value that will be assigned in the mailbox..
	NextID() uint64

	// LastID returns the highest index value in the mailbox, followed by a
	// map containing the last indices for each suffix.
	LastID() (uint64, map[uint64]uint64)
}

// Folders represents a set of named folders.
type Folders interface {
	New(string) (Folder, error)
	Get(string) (Folder, error)
	Rename(string, string) error
	Names() []string
	Delete(string) error
}

type message struct {
	payload []byte
	suffix  uint64
}

// memFolder is a folder that exists in memory rather than in bolt db.
// Not currently used but possibly useful in the future.
type memFolder struct {
	lastIndex         uint64
	nextIndex         uint64
	lastIndexBySuffix map[uint64]uint64
	messages          map[uint64]message
}

// newMemFolder returns an in-memory folder.
func newMemFolder() *memFolder {
	return &memFolder{
		lastIndex:         0,
		nextIndex:         1,
		messages:          make(map[uint64]message),
		lastIndexBySuffix: make(map[uint64]uint64),
	}
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
		f.nextIndex++
	}()

	f.messages[f.nextIndex] = message{payload: msg, suffix: suffix}

	return f.nextIndex, nil
}

func (f *memFolder) InsertMessage(id uint64, msg []byte, suffix uint64) error {
	if id == 0 || id >= f.nextIndex {
		return ErrInvalidID
	}

	if msg == nil {
		return errors.New("Nil message inserted.")
	}

	if _, ok := f.messages[id]; ok {
		return ErrDuplicateID
	}

	f.updateLast(id, suffix)

	f.messages[id] = message{payload: msg, suffix: suffix}

	return nil
}

func (f *memFolder) GetMessage(id uint64) (uint64, []byte, error) {
	if id == 0 || id >= f.nextIndex {
		return 0, nil, ErrNotFound
	}

	m, ok := f.messages[id]
	if !ok {
		return 0, nil, ErrNotFound
	}

	return m.suffix, m.payload, nil
}

func (f *memFolder) DeleteMessage(id uint64) error {
	if id == 0 || id >= f.nextIndex {
		return ErrInvalidID
	}

	var suffix uint64
	m, ok := f.messages[id]
	if !ok {
		return ErrNotFound
	}

	suffix = m.suffix

	delete(f.messages, id)

	if f.lastIndexBySuffix[suffix] != id {
		return nil
	}

	for k := id - 1; k > 0; k-- {

		if m, ok := f.messages[k]; ok {
			if id == f.lastIndex {
				f.lastIndex = k
			}

			if m.suffix == suffix {
				f.lastIndexBySuffix[suffix] = k
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

	for id, m := range f.messages {
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

type memFolders struct {
	folders map[string]*memFolder
}

// NewMemFolders returns an in-memory folders object.
func NewMemFolders() Folders {
	return &memFolders{
		folders: make(map[string]*memFolder),
	}
}

func (mf *memFolders) New(name string) (Folder, error) {
	_, ok := mf.folders[name]

	if ok {
		return nil, ErrDuplicateID
	}

	f := newMemFolder()

	mf.folders[name] = f

	return f, nil
}

func (mf *memFolders) Get(name string) (Folder, error) {
	f, ok := mf.folders[name]

	if !ok {
		return nil, ErrNotFound
	}

	return f, nil
}

func (mf *memFolders) Rename(name, newname string) error {
	f, ok := mf.folders[name]

	if !ok {
		return ErrNotFound
	}

	mf.folders[newname] = f

	return nil
}

func (mf *memFolders) Delete(name string) error {
	_, ok := mf.folders[name]

	if !ok {
		return ErrNotFound
	}

	delete(mf.folders, name)

	return nil
}

func (mf *memFolders) Names() []string {
	names := make([]string, 0, len(mf.folders))

	for name := range mf.folders {
		names = append(names, name)
	}

	return names
}
