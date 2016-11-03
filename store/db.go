// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	"github.com/DanielKrawisz/bmutil/wire"
	"golang.org/x/crypto/nacl/secretbox"
	"golang.org/x/crypto/pbkdf2"
)

const (
	// dbTimeout is the time duration after which an attempted connection to the
	// database must time out.
	dbTimeout = time.Millisecond * 5

	// nonceSize is the size of the nonce (in bytes) used by secretbox.
	nonceSize = 24

	// saltLength is the desired length of salt used by PBKDF2.
	saltLength = 32

	// keySize is the size of the symmetric key for use with secretbox.
	keySize = 32

	// numIters is the number of iterations to be done by PBKDF2.
	numIters = 1 << 15

	// latestStoreVersion is the most recent version of data store. This is how
	// Store can know whether to update the database structure or not.
	latestStoreVersion = 0x01
)

// Buckets for storing data in the database.
var (
	powQueueBucket           = []byte("powQueue")
	pkRequestsBucket         = []byte("pubkeyRequests")
	miscBucket               = []byte("misc")
	countersBucket           = []byte("counters")
	broadcastAddressesBucket = []byte("broadcastAddresses")
	foldersBucket            = []byte("folders")
	usersBucket              = []byte("users")

	// Bucket is a sub-bucket of "folders"
	folderDataBucket = []byte("data")
	
	userPrefix = []byte("user:")

	// Used for storing the encrypted master key used for encryption/decryption.
	dbMasterKeyEnc = []byte("dbMasterKeyEnc")

	// Used for PBKDF2, to generate key used to decrypt master key.
	saltKey = []byte("salt")

	// Version of the data store.
	versionKey = []byte("version")

	// folderNextIDKey contains the index of the next element. Exists because
	// IMAP requires existence of unique message IDs that do not change over
	// sessions.
	folderNextIDKey = []byte("folderNextID")

	// powQueueLatestIDKey contains the index of the last element in the POW
	// queue.
	powQueueLatestIDKey = []byte("powQueueLatestID")
	
	usersLatestIDKey = []byte("usersLatestIDKey")
)

var (
	// ErrNotFound is returned when a record matching the query or no record at
	// all is found in the database.
	ErrNotFound = errors.New("record not found")

	// ErrDecryptionFailed is returned when decryption of the master key fails.
	// This could be due to invalid passphrase or corrupt/tampered data.
	ErrDecryptionFailed = errors.New("invalid passphrase")

	// ErrDuplicateMailbox is returned by NewMailbox when a mailbox with the
	// given name already exists.
	ErrDuplicateMailbox = errors.New("duplicate mailbox")
)

// Type for transforming underlying database into Store. (used to abstract
// the details of the underlying bolt db).
type Loader struct {
	db        *bolt.DB
}

// Open creates a new Store from the given file.
func Open(file string) (*Loader, error) {
	db, err := bolt.Open(file, 0600, &bolt.Options{Timeout: dbTimeout})
	if err != nil {
		return nil, err
	}
	
	return &Loader{
		db: db,
	}, nil
}

// Close performs any necessary cleanups and then closes the database.
func (l *Loader) Close() error {
	if l.db == nil {
		return nil
	}
	
	defer func() {
		l.db = nil
	}()
	return l.db.Close()
}

// Store persists all information about public key requests, pending POW,
// incoming/outgoing/pending messages to disk.
type Store struct {
	masterKey   *[keySize]byte      // can be nil.
	db          *bolt.DB
	mutex       sync.RWMutex        // For protecting the map.
	Users       map[string]*UserData
}

// deriveKey is used to derive a 32 byte key for encryption/decryption
// operations with secretbox. It runs a large number of rounds of PBKDF2 on the
// password using the specified salt to arrive at the key.
func deriveKey(pass, salt []byte) *[keySize]byte {
	out := pbkdf2.Key(pass, salt, numIters, keySize, sha256.New)
	var key [keySize]byte
	copy(key[:], out)
	return &key
}

func (l *Loader) IsEncrypted() bool {
	if l.db == nil {
		return false
	}
	
	tx, err := l.db.Begin(false)
	if err != nil {
		l.Close()
		return false
	}
	
	defer tx.Rollback()
	
	bucket := tx.Bucket(miscBucket)
	
	return bucket != nil && bucket.Get(dbMasterKeyEnc) != nil
}

// Construct creates a new Store from the given file.
func (l *Loader) Construct(pass []byte) (*Store, *PKRequests, error) {
	
	if l.db == nil {
		return nil, nil, errors.New("Closed database.");
	}
	
	if pass == nil {
		clientLog.Warn("Unencrypted database opened.")
	}
	
	var masterKey [keySize]byte
	
	// Verify passphrase, or create it if necessary.
	err := l.db.Update(func(tx *bolt.Tx) error {
		
		misc, err := tx.CreateBucketIfNotExists(miscBucket)
		if err != nil {
			return err
		}
		_, err = misc.CreateBucketIfNotExists(countersBucket)
		if err != nil {
			return err
		}
		
		_, err = tx.CreateBucketIfNotExists(usersBucket)
		if err != nil {
			return err
		}
		
		bVersion := misc.Get(versionKey)
		if bVersion == nil { // This is a new database.
			if (pass != nil) {
				
				// Generate master key.
				_, err := rand.Read(masterKey[:])
				if err != nil {
					return err
				}
	
				// Encrypt master key.
				salt, v, err := encryptKey(masterKey, pass)
	
				// Store salt in database.
				err = misc.Put(saltKey, salt)
				if err != nil {
					return err
				}
	
				// Store encrypted master key in database.
				err = misc.Put(dbMasterKeyEnc, v)
				if err != nil {
					return err
				}
			}

			// Set database version.
			err = misc.Put(versionKey, []byte{latestStoreVersion})
			if err != nil {
				return err
			}

			// Set ID for PoW queue to 0.
			err = misc.Put(powQueueLatestIDKey, []byte{0, 0, 0, 0, 0, 0, 0, 0})
			if err != nil {
				return err
			}
			
			return misc.Put(usersLatestIDKey, []byte{0, 0, 0, 0, 0, 0, 0, 0})
		
		}
		
		// Check if upgrade is required. 
		if bVersion[0] != latestStoreVersion {
			err = upgrade(tx)
			if err != nil {
				return err;
			}
		}

		v := misc.Get(dbMasterKeyEnc)
		if v == nil {
			if pass != nil {
				return errors.New("Password given for unencrypted database.")
			}
		} else {
			if pass == nil {
				return errors.New("No password supplied for encrypted database.")
			}
			
			var nonce [nonceSize]byte
		
			if len(v) < nonceSize+keySize+secretbox.Overhead {
				return errors.New("Encrypted master key too short.")
			}
			
			copy(nonce[:], v[:nonceSize])
			salt := misc.Get(saltKey)
			key := deriveKey(pass, salt)
	
			mKey, success := secretbox.Open(nil, v[nonceSize:], &nonce, key)
			if !success {
				return ErrDecryptionFailed
			}
	
			// Store decrypted master key in memory.
			copy(masterKey[:], mKey)
		}
		
		return nil
	})
	
	if err != nil {
		l.Close()
		return nil, nil, err
	}
	
	store := &Store{
		db:        l.db,
		Users: make(map[string]*UserData),
		masterKey: &masterKey,
	}

	err = initializePKRequestStore(l.db)
	if err != nil {
		l.Close()
		return nil, nil, err
	}
	
	if err != nil {
		l.Close()
		return nil, nil, err
	}
	
	// Load existing users.
	users := make([]string, 0)
	err = l.db.View(func(tx *bolt.Tx) error {
		
		return tx.Bucket(usersBucket).ForEach(func(name, _ []byte) error {
			users = append(users, string(name))
			return nil
		})
	})
	if err != nil {
		l.Close()
		return nil, nil, err
	}
	
	for _, name := range users {
		store.addUser(name)
	}

	return store, &PKRequests{db:l.db}, nil
}

func (s *Store) addUser(username string) (*UserData, error) {
	uname := append(userPrefix, []byte(username)...)
	
	folders, err := s.initializeFolders(username, uname)
	if err != nil {
		return nil, err
	}
		
	broadcast, err := newBroadcastsStore(s.db, username)
	if err != nil {
		s.Close()
		return nil, err
	}
	
	user := newUserData(
		s.masterKey, 
		s.db, 
		uname, 
		username, 
		broadcast, 
		folders)
	
	s.Users[username] = user
	
	return user, nil
}

func (s *Store) initializeFolders(username string, uname []byte) (map[string]struct{}, error) {

	// Verify passphrase, or create it if necessary.
	err := s.db.Update(func(tx *bolt.Tx) error {
		
		newUser := false
		var err error
		
		userBucket := tx.Bucket(uname)
		if userBucket == nil {
			
			userBucket, err = tx.CreateBucket(uname)
			if err != nil {
				return err
			}
			
			users, err := tx.CreateBucketIfNotExists(usersBucket)		
			if err != nil {
				return err
			}	
			misc, err := tx.CreateBucketIfNotExists(miscBucket)
			if err != nil {
				return err
			}
			
			index := binary.BigEndian.Uint64(misc.Get(usersLatestIDKey)) + 1
			k := make([]byte, 8)
			binary.BigEndian.PutUint64(k, index)
			
			users.Put([]byte(username), k)
			misc.Put(usersLatestIDKey, k)
			
			newUser = true	
		}
		
		misc, err := userBucket.CreateBucketIfNotExists(miscBucket)
		if err != nil {
			return err
		}
		_, err = userBucket.CreateBucketIfNotExists(foldersBucket)
		if err != nil {
			return err
		}
		
		if newUser { // This is a new user.
			// Set ID for messages to 0.
			// Only one id is used for the entire set of mailboxes for a given user.
			err = misc.Put(folderNextIDKey, []byte{0, 0, 0, 0, 0, 0, 0, 0})
			if err != nil {
				return err
			}
		}
		
		return nil
	})
	
	if err != nil {
		s.Close()
		return nil, err
	}

	// Load existing mailboxes.
	folders := make(map[string]struct{})
	err = s.db.View(func(tx *bolt.Tx) error {
		
		return tx.Bucket(uname).Bucket(foldersBucket).ForEach(func(name, _ []byte) error {
			folders[string(name)] = struct{}{}
			return nil
		})
	})
	if err != nil {
		s.Close()
		return nil, err
	}
	
	return folders, nil
}

func (s *Store) GetUser(name string) (*UserData, error) {
	u, ok := s.Users[name]
	if !ok || u == nil {
		return nil, errors.New("No such user.")
	}
	return u, nil
}

func (s *Store) NewUser(name string) (*UserData, error) {
	_, ok := s.Users[name]
	if ok {
		return nil, errors.New("Duplicate user.")
	}
	
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	return s.addUser(name)
}

// Close performs any necessary cleanups and then closes the store.
func (s *Store) Close() error {
	return s.db.Close()
}

// upgrade is responsible for checking the version of the data store
// and upgrading itself if necessary.
func upgrade(tx *bolt.Tx) error {
	bVersion := tx.Bucket(miscBucket).Get(versionKey)
	if bVersion[0] != latestStoreVersion {
		return errors.New("Unrecognized version of data store.")
	}
	return nil
}

// ChangePassphrase changes the passphrase of the data store. It does not
// protect against a previous compromise of the data file. Refer to package docs
// for more details.
func (s *Store) ChangePassphrase(pass []byte) error {
	if (s.masterKey == nil) {
		return errors.New("Database is not encrypted.")
	}
	
	// Encrypt master key.
	salt, v, err := encryptKey(*s.masterKey, pass)

	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(miscBucket)

		// Store salt in database.
		err = bucket.Put(saltKey, salt)
		if err != nil {
			return err
		}

		// Store encrypted master key in database.
		err = bucket.Put(dbMasterKeyEnc, v)
		if err != nil {
			return err
		}

		return nil
	})
}

// GetCounter returns the stored counter value associated with the given object
// type.
func (s *Store) GetCounter(objType wire.ObjectType) (uint64, error) {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(objType))
	var res uint64

	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(miscBucket).Bucket(countersBucket).Get(b)
		if v == nil { // Counter doesn't exist so just return 1.
			res = 1
		} else {
			res = binary.BigEndian.Uint64(v)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return res, nil
}

// SetCounter sets the counter value associated with the given object type.
func (s *Store) SetCounter(objType wire.ObjectType, counter uint64) error {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(objType))

	bc := make([]byte, 8)
	binary.BigEndian.PutUint64(bc, counter)

	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(miscBucket).Bucket(countersBucket).Put(b, bc)
	})
}

// encryptKey is a helper function that encrypts the master key with the
// given passphrase. It generates the encryption key using PBKDF2 and returns
// the salt used and the encrypted data.
func encryptKey(masterKey [keySize]byte, pass []byte) ([]byte, []byte, error) {
	salt := make([]byte, saltLength)
	var nonce [nonceSize]byte

	// Read random nonce and salt.
	_, err := rand.Read(nonce[:])
	if err != nil {
		return nil, nil, err
	}
	_, err = rand.Read(salt)
	if err != nil {
		return nil, nil, err
	}

	// Key used to encrypt the master key.
	key := deriveKey(pass, salt)

	// Encrypt the master key.
	enc := make([]byte, nonceSize)
	copy(enc[:nonceSize], nonce[:])
	enc = secretbox.Seal(enc, masterKey[:], &nonce, key)

	return salt, enc, nil
}

// encrypt encrypts the data using nacl.Secretbox with the master key. It
// generates a random nonce and prepends to the output.
func encrypt(masterKey *[keySize]byte, db *bolt.DB, data []byte) ([]byte, error) {
	if (masterKey == nil) {
		return data, nil
	}
	
	// Generate a random nonce
	var nonce [nonceSize]byte
	_, err := rand.Read(nonce[:])
	if err != nil {
		return nil, err
	}

	enc := make([]byte, nonceSize)
	copy(enc[:nonceSize], nonce[:])

	return secretbox.Seal(enc, data, &nonce, masterKey), nil
}

// decrypt undoes the operation done by encrypt. It takes the prepended nonce
// and decrypts what follows with the master key.
func decrypt(masterKey *[keySize]byte, db *bolt.DB, data []byte) ([]byte, bool) {
	if (masterKey == nil) {
		return data, true
	}
	
	// Read nonce
	var nonce [nonceSize]byte
	copy(nonce[:], data[:nonceSize])

	return secretbox.Open(nil, data[nonceSize:], &nonce, masterKey)
}
