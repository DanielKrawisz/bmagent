// Copyright (c) 2015 Monetas.
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
	"github.com/monetas/bmutil/wire"
	"golang.org/x/crypto/nacl/secretbox"
	"golang.org/x/crypto/pbkdf2"
)

// MailboxType is the type of mailbox.
type MailboxType uint8

const (
	// MailboxPrivate is mailbox for private messages. A mailbox is considered
	// private when only one person holds the private signing and encryption
	// keys.
	MailboxPrivate MailboxType = iota

	// MailboxBroadcast is a mailbox for broadcasst. A broadcast can be
	// decrypted by anyone who knows the sender's address.
	MailboxBroadcast

	// MailboxChannel is a mailbox for a channel subscription. A channel is a
	// deterministically generated identity (from a passphrase). Anyone who
	// knows the passphrase can generate the private signing and encryption
	// keys. Thus, multiple people can use channels to communicate securely;
	// effectively turning them into mailing lists.
	MailboxChannel

	// mailboxSent is the mailbox used for sent messages. Messages stored here
	// could originate from any address and be destined for any address.
	mailboxSent

	// mailboxPending is the mailbox used for pending messages. Messages stored
	// here need either the public key of the recipient or proof-of-work to be
	// done on them.
	mailboxPending
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
	powQueueBucket   = []byte("powQueue")
	pkRequestsBucket = []byte("pubkeyRequests")
	miscBucket       = []byte("misc")
	countersBucket   = []byte("counters")
	mailboxesBucket  = []byte("mailboxes")

	// Addresses used for creating special mailboxes.
	sentMailboxAddr    = "Sent"
	pendingMailboxAddr = "Pending"
	mailboxDataBucket  = []byte("data") // Bucket is a sub-bucket of "mailboxes"

	// Used for storing the encrypted master key used for encryption/decryption.
	dbMasterKeyKey = []byte("dbMasterKey")

	// Used for PBKDF2, to generate key used to decrypt master key.
	saltKey = []byte("salt")

	// Version of the data store.
	versionKey = []byte("version")
)

var (
	// ErrNotFound is returned when a record matching the query or no record at
	// all is found in the database.
	ErrNotFound = errors.New("record not found")

	// ErrDecryptionFailed is returned when decryption of the master key fails.
	// This could be due to invalid passphrase or corrupt/tampered data.
	ErrDecryptionFailed = errors.New("invalid passphrase")

	// ErrDuplicateName is returned by SetName when another mailbox of the same
	// type already has the provided name.
	ErrDuplicateName = errors.New("duplicate name")

	// ErrDuplicateMailbox is returned by NewMailbox when a mailbox with the
	// given address already exists.
	ErrDuplicateMailbox = errors.New("duplicate mailbox")
)

// Store persists all information about public key requests, pending POW,
// incoming/outgoing/pending messages to disk.
type Store struct {
	masterKey      *[keySize]byte
	db             *bolt.DB
	PubkeyRequests *PKRequests
	PowQueue       *PowQueue
	mutex          sync.RWMutex        // For protecting the map.
	mailboxes      map[string]*Mailbox // Map addresses to all mailboxes.
	broadcastAddrs map[string]struct{} // All broadcast addresses.
	SentMailbox    *Mailbox
	PendingMailbox *Mailbox
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

// Open creates a new Store from the given file.
func Open(file string, pass []byte) (*Store, error) {
	db, err := bolt.Open(file, 0600, &bolt.Options{Timeout: dbTimeout})
	if err != nil {
		return nil, err
	}
	store := &Store{
		db:             db,
		mailboxes:      make(map[string]*Mailbox),
		broadcastAddrs: make(map[string]struct{}),
	}

	// Verify passphrase, or create it, if necessary.
	err = db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(miscBucket)
		if err != nil {
			return err
		}
		_, err = bucket.CreateBucketIfNotExists(countersBucket)
		if err != nil {
			return err
		}

		var masterKey [keySize]byte
		var nonce [nonceSize]byte

		v := bucket.Get(dbMasterKeyKey)
		if v == nil { // It's a new database.

			// Generate master key.
			_, err := rand.Read(masterKey[:])
			if err != nil {
				return err
			}

			// Store master key in store.
			store.masterKey = &masterKey

			// Encrypt master key.
			salt, v, err := store.encryptMasterKey(pass)

			// Store salt in database.
			err = bucket.Put(saltKey, salt)
			if err != nil {
				return err
			}

			// Store encrypted master key in database.
			err = bucket.Put(dbMasterKeyKey, v)
			if err != nil {
				return err
			}

			// Set database version.
			err = bucket.Put(versionKey, []byte{latestStoreVersion})
			if err != nil {
				return err
			}

			return nil

		} else if len(v) < nonceSize+keySize+secretbox.Overhead {
			return errors.New("Encrypted master key too short.")
		}

		// We're opening an existing database.
		copy(nonce[:], v[:nonceSize])
		salt := bucket.Get(saltKey)
		key := deriveKey(pass, salt)

		mKey, success := secretbox.Open(nil, v[nonceSize:], &nonce, key)
		if !success {
			return ErrDecryptionFailed
		}

		// Store decrypted master key in memory.
		copy(masterKey[:], mKey)
		store.masterKey = &masterKey

		// Upgrade database if necessary.
		return store.checkAndUpgrade(tx)
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	store.PubkeyRequests, err = newPKRequestStore(store)
	if err != nil {
		db.Close()
		return nil, err
	}

	store.PowQueue, err = newPowQueue(store)
	if err != nil {
		db.Close()
		return nil, err
	}

	// Create bucket needed for mailboxes.
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(mailboxesBucket)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	// Load existing mailboxes.
	mailboxes := make(map[string]MailboxType)
	err = db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(mailboxesBucket).ForEach(func(address, _ []byte) error {
			switch string(address) {
			// Ignore these, because they are special mailboxes.
			case sentMailboxAddr:
			case pendingMailboxAddr:
			default:
				mailboxes[string(address)] = MailboxType(tx.Bucket(mailboxesBucket).
					Bucket(address).Bucket(mailboxDataBucket).Get(mailboxTypeKey)[0])
			}
			return nil
		})
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	for address, boxType := range mailboxes {
		store.mailboxes[address], err = newMailbox(store, address, boxType)
		if err != nil {
			db.Close()
			return nil, err
		}
		// Load broadcast mailboxes.
		if boxType == MailboxBroadcast {
			store.broadcastAddrs[address] = struct{}{}
		}
	}

	// Load special mailboxes.
	store.SentMailbox, err = newMailbox(store, sentMailboxAddr, mailboxSent)
	if err != nil {
		db.Close()
		return nil, err
	}
	store.PendingMailbox, err = newMailbox(store, pendingMailboxAddr, mailboxPending)
	if err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

// Close performs any necessary cleanups and then closes the store.
func (s *Store) Close() error {
	return s.db.Close()
}

// checkAndUpgrade is responsible for checking the version of the data store
// and upgrading itself if necessary.
func (s *Store) checkAndUpgrade(tx *bolt.Tx) error {
	bVersion := tx.Bucket(miscBucket).Get(versionKey)
	if bVersion[0] != latestStoreVersion {
		return errors.New("Unrecognized version of data store.")
	}
	return nil
}

// NewMailbox creates a new mailbox for an address. It takes the address,
// MailboxType and a name for the mailbox. Address must be unique. Name must
// be unique for a MailboxType.
func (s *Store) NewMailbox(address string, boxType MailboxType,
	name string) (*Mailbox, error) {

	// Check if address exists.
	_, ok := s.mailboxes[address]
	if ok {
		return nil, ErrDuplicateMailbox
	}

	// We're good, so create the mailbox.
	mbox, err := newMailbox(s, address, boxType)
	if err != nil {
		return nil, err
	}

	// Save mailbox in the local map.
	s.mutex.Lock()
	s.mailboxes[address] = mbox
	if boxType == MailboxBroadcast {
		s.broadcastAddrs[address] = struct{}{}
	}
	s.mutex.Unlock()

	err = mbox.SetName(name)
	if err != nil {
		// We still want to return the mailbox so that the user can set a
		// different name.
		return mbox, err
	}

	return mbox, nil
}

// MailboxByAddress retrieves the mailbox associated with an address. If the
// mailbox doesn't exist, an error is returned.
func (s *Store) MailboxByAddress(address string) (*Mailbox, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	mbox, ok := s.mailboxes[address]
	if !ok {
		return nil, ErrNotFound
	}
	return mbox, nil
}

// ForEachBroadcastAddress runs the provided function for each broadcast address
// that the user is subscribed to, breaking early on error.
func (s *Store) ForEachBroadcastAddress(f func(address string) error) error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for addr := range s.broadcastAddrs {
		if err := f(addr); err != nil {
			return err
		}
	}
	return nil
}

// ChangePassphrase changes the passphrase of the data store. It does not
// protect against a previous compromise of the data file. Refer to package docs
// for more details.
func (s *Store) ChangePassphrase(pass []byte) error {
	// Encrypt master key.
	salt, v, err := s.encryptMasterKey(pass)

	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(miscBucket)

		// Store salt in database.
		err = bucket.Put(saltKey, salt)
		if err != nil {
			return err
		}

		// Store encrypted master key in database.
		err = bucket.Put(dbMasterKeyKey, v)
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

// encryptMasterKey is a helper function that encrypts the master key with the
// given passphrase. It generates the encryption key using PBKDF2 and returns
// the salt used and the encrypted data.
func (s *Store) encryptMasterKey(pass []byte) ([]byte, []byte, error) {
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
	enc = secretbox.Seal(enc, s.masterKey[:], &nonce, key)

	return salt, enc, nil
}

// encrypt encrypts the data using nacl.Secretbox with the master key. It
// generates a random nonce and prepends to the output.
func (s *Store) encrypt(data []byte) ([]byte, error) {
	// Generate a random nonce
	var nonce [nonceSize]byte
	_, err := rand.Read(nonce[:])
	if err != nil {
		return nil, err
	}

	enc := make([]byte, nonceSize)
	copy(enc[:nonceSize], nonce[:])

	return secretbox.Seal(enc, data, &nonce, s.masterKey), nil
}

// decrypt undoes the operation done by encrypt. It takes the prepended nonce
// and decrypts what follows with the master key.
func (s *Store) decrypt(data []byte) ([]byte, bool) {
	// Read nonce
	var nonce [nonceSize]byte
	copy(nonce[:], data[:nonceSize])

	return secretbox.Open(nil, data[nonceSize:], &nonce, s.masterKey)
}
