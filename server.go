// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/aes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/monetas/bmclient/keymgr"
	"github.com/monetas/bmclient/rpc"
	"github.com/monetas/bmclient/store"
	"github.com/monetas/bmutil"
	"github.com/monetas/bmutil/cipher"
	"github.com/monetas/bmutil/identity"
	"github.com/monetas/bmutil/pow"
	"github.com/monetas/bmutil/wire"
)

const (
	// pkCheckerInterval is the interval after which bmclient should query bmd
	// for retrieving any new public identities (that we queried for before).
	pkCheckerInterval = time.Minute * 5

	// powCheckerInterval is the interval after with bmclient should check the
	// proof-of-work queue and process next item.
	powCheckerInterval = time.Second * 10

	// saveInterval is the interval after which data in memory should be saved
	// to disk.
	saveInterval = time.Minute * 5

	// pubkeyExpiry is the time after which a pubkey sent out to the network
	// expires.
	pubkeyExpiry = time.Hour * 24 * 14

	// getpubkeyExpiry is the time after which a getpubkey request sent out to
	// the network expires.
	getpubkeyExpiry = time.Hour * 24 * 14
)

// server struct manages everything that a running instance of bmclient
// comprises of and would need.
type server struct {
	bmd              *rpc.Client
	keymgr           *keymgr.Manager
	store            *store.Store
	started          int32
	shutdown         int32
	msgCounter       uint64
	broadcastCounter uint64
	getpubkeyCounter uint64
	quit             chan struct{}
	wg               sync.WaitGroup
}

// newServer initializes a new instance of server.
func newServer(bmd *rpc.Client, kmgr *keymgr.Manager,
	s *store.Store) (*server, error) {
	srvr := &server{
		bmd:    bmd,
		keymgr: kmgr,
		store:  s,
		quit:   make(chan struct{}),
	}
	bmd.SetHandlers(srvr.newMessage, srvr.newBroadcast, srvr.newGetpubkey)

	// Load counter values from store.
	var err error

	srvr.msgCounter, err = s.GetCounter(wire.ObjectTypeMsg)
	if err != nil {
		serverLog.Critical("Failed to get message counter:", err)
	}

	srvr.broadcastCounter, err = s.GetCounter(wire.ObjectTypeBroadcast)
	if err != nil {
		serverLog.Critical("Failed to get broadcast counter:", err)
	}

	srvr.getpubkeyCounter, err = s.GetCounter(wire.ObjectTypeGetPubKey)
	if err != nil {
		serverLog.Critical("Failed to get getpubkey counter:", err)
	}

	return srvr, nil
}

// Start starts all the servers one by one and returns an error if any fails.
func (s *server) Start() {
	// Already started?
	if atomic.AddInt32(&s.started, 1) != 1 {
		return
	}

	// Start RPC client.
	serverLog.Info("Starting RPC client handlers.")
	s.bmd.Start(s.msgCounter, s.broadcastCounter, s.getpubkeyCounter)

	// Start IMAP server.

	// Start SMTP server.

	// Start RPC server.

	// Start public key request handler.
	serverLog.Info("Starting public key request handler.")
	s.wg.Add(1)
	go s.pkRequestHandler()

	// Start proof of work handler.
	serverLog.Info("Starting proof-of-work handler.")
	s.wg.Add(1)
	go s.powHandler()

	// Start saving data periodically.
	s.wg.Add(1)
	go s.savePeriodically()
}

// newMessage is called when a new message is received by the RPC client.
// Messages are guaranteed to be received in ascending order of counter value.
func (s *server) newMessage(counter uint64, obj []byte) {
	// Store counter value.
	atomic.StoreUint64(&s.msgCounter, counter)

	msg := &wire.MsgMsg{}
	err := msg.Decode(bytes.NewReader(obj))
	if err != nil {
		serverLog.Errorf("Failed to decode message #%d from bytes: %v", counter,
			err)
		return // Ignore message.
	}

	// Check if message is smaller than expected.
	// IV + Curve params/X/Y + 1 block + HMAC-256
	if len(msg.Encrypted) <= aes.BlockSize+70+aes.BlockSize+sha256.Size {
		return
	}

	// Contains the address of the identity used to decrypt the message.
	var address string

	// Try decrypting with all available identities.
	err = s.keymgr.ForEach(func(id *keymgr.PrivateID) error {
		if cipher.TryDecryptAndVerifyMsg(msg, &id.Private) == nil {
			address, _ = id.Address.Encode()
			return errors.New("decryption successful")
		}
		return nil
	})
	if err == nil {
		// Decryption unsuccessful.
		return
	}

	// Decryption was successful. Add message to store.
	_, err = s.store.MailboxByAddress(address)
	if err != nil {
		serverLog.Errorf("Failed to get mailbox for %v", address)
		return
	}

	from := bmutil.Address{
		Version: msg.FromAddressVersion,
		Stream:  msg.FromStreamNumber,
		Ripe:    *msg.Destination,
	}
	fromAddress, err := from.Encode()
	if err != nil {
		fromAddress = "INVALID"
		log.Errorf("Invalid sender of message #%d (address encode failure): %v",
			counter, err)
	}

	//mbox.InsertMessage(msg.Message, msg.Encoding)
	log.Infof("Got new message from %s:\n%s", fromAddress, string(msg.Message))

	// Send out ack if necessary.
	ack := &wire.MsgObject{}
	err = ack.Decode(bytes.NewReader(msg.Ack))
	if err != nil { // Can't send invalid Ack.
		return
	}
	_, err = s.bmd.SendObject(msg.Ack)
	if err != nil {
		log.Infof("Failed to send ack for message #%d: %v", counter, err)
		return
	}
}

// newBroadcast is called when a new broadcast is received by the RPC client.
// Broadcasts are guaranteed to be received in ascending order of counter value.
func (s *server) newBroadcast(counter uint64, obj []byte) {
	// Store counter value.
	atomic.StoreUint64(&s.broadcastCounter, counter)

	msg := &wire.MsgBroadcast{}
	err := msg.Decode(bytes.NewReader(obj))
	if err != nil {
		serverLog.Errorf("Failed to decode broadcast #%d from bytes: %v",
			counter, err)
		return // Ignore message.
	}

	// Check if broadcast is smaller than expected.
	// IV + Curve params/X/Y + 1 block + HMAC-256
	if len(msg.Encrypted) <= aes.BlockSize+70+aes.BlockSize+sha256.Size {
		return
	}

	var fromAddress string

	err = s.store.ForEachBroadcastAddress(func(address string) error {
		addr, err := bmutil.DecodeAddress(address)
		if err != nil { // BUG
			serverLog.Critical("Got error decoding address:", err)
			return nil
		}
		if cipher.TryDecryptAndVerifyBroadcast(msg, addr) == nil {
			fromAddress = address
			return errors.New("Broadcast decryption succeeded.")
		}
		return nil
	})
	if err == nil { // Broadcast decryption failed.
		return
	}

	//mbox.InsertMessage(msg.Message, msg.Encoding)
	serverLog.Infof("Got new broadcast from %s:\n%s", fromAddress, msg.Message)
}

// newGetpubkey is called when a new getpubkey is received by the RPC client.
// Getpubkey requests are guaranteed to be received in ascending order of
// counter value.
func (s *server) newGetpubkey(counter uint64, obj []byte) {
	// Store counter value.
	atomic.StoreUint64(&s.getpubkeyCounter, counter)

	msg := &wire.MsgGetPubKey{}
	err := msg.Decode(bytes.NewReader(obj))
	if err != nil {
		serverLog.Errorf("Failed to decode getpubkey #%d from bytes: %v",
			counter, err)
		return // Ignore message.
	}

	var privID *keymgr.PrivateID

	// Check if the getpubkey request corresponds to any of our identities.
	err = s.keymgr.ForEach(func(id *keymgr.PrivateID) error {
		if id.Disabled || id.IsChan { // We don't care about these.
			return nil
		}

		var cond bool // condition to satisfy
		switch msg.Version {
		case wire.SimplePubKeyVersion, wire.ExtendedPubKeyVersion:
			cond = bytes.Equal(id.Address.Ripe[:], msg.Ripe[:])
		case wire.TagGetPubKeyVersion:
			cond = bytes.Equal(id.Address.Tag(), msg.Tag[:])
		}
		if cond {
			privID = id
			return errors.New("We have a match.")
		}
		return nil
	})
	if err == nil {
		return
	}

	// Generate a pubkey message.
	pkMsg, err := cipher.GeneratePubKey(&privID.Private, pubkeyExpiry)
	if err != nil {
		addr, _ := privID.Address.Encode()
		serverLog.Errorf("Failed to generate pubkey for %s: %v", addr, err)
		return
	}

	// Add it to POW queue.
	b := wire.EncodeMessage(pkMsg)[8:] // exclude nonce
	target := pow.CalculateTarget(uint64(len(b)),
		uint64(pkMsg.ExpiresTime.Sub(time.Now()).Seconds()),
		pow.DefaultNonceTrialsPerByte, pow.DefaultExtraBytes)

	_, err = s.store.PowQueue.Enqueue(target, b)
	if err != nil {
		serverLog.Critical("Failed to enqueue pow request:", err)
		return
	}
}

// pkRequestHandler manages the pubkey request store. It periodically checks
// with bmd whether the requested identities have been received. If they have,
// it removes them from the pubkey request store and processes messages that
// need that identity.
func (s *server) pkRequestHandler() {
	defer s.wg.Done()
	t := time.NewTicker(pkCheckerInterval)

	for {
		select {
		case <-s.quit:
			return
		case <-t.C:
			// Go through our store and check if server has any new public
			// identity.
			s.store.PubkeyRequests.ForEach(func(address string, addTime time.Time) {
				_, err := s.bmd.GetIdentity(address)
				if err == rpc.ErrIdentityNotFound {
					// TODO check whether addTime has exceeded a set constant.
					// If it has, delete the message from queue and generate a
					// bounce message.
					return
				} else if err != nil {
					rpccLog.Errorf("GetIdentity gave on %s unexpected error %v",
						address, err)
					return
				}

				// Now that we have the public identity, remove it from the
				// PK request store.
				err = s.store.PubkeyRequests.Remove(address)
				if err != nil {
					serverLog.Criticalf("Failed to remove address from public"+
						" key request store: %v", err)
				}

				// TODO process pending messages with this public identity and
				// add them to pow queue.

			})
		}
	}
}

// powHandler manages the proof-of-work queue. It makes sure that only one
// object is processed at a time. After doing POW, it sends the object out on
// the network.
func (s *server) powHandler() {
	defer s.wg.Done()
	t := time.NewTicker(powCheckerInterval)

	for {
		select {
		case <-s.quit:
			return
		case <-t.C:
			target, hash, err := s.store.PowQueue.PeekForPow()
			if err == nil { // We have something to process
				nonce := cfg.powHandler(target, hash)

				// Since we have the required nonce value and have processed
				// the pending message, remove it from the queue.
				_, obj, err := s.store.PowQueue.Dequeue()
				if err != nil {
					serverLog.Criticalf("Dequeue on PowQueue failed: %v", err)
					continue
				}

				// Re-assemble message as bytes.
				nonceBytes := make([]byte, 8)
				binary.BigEndian.PutUint64(nonceBytes, nonce)
				obj = append(nonceBytes, obj...)

				// Create MsgObject.
				msg := &wire.MsgObject{}
				err = msg.Decode(bytes.NewReader(obj))
				if err != nil {
					serverLog.Criticalf("MsgObject.Decode failed: %v", err)
					continue
				}

				// TODO take appropriate actions for messages in various folders
				if msg.ObjectType == wire.ObjectTypeMsg ||
					msg.ObjectType == wire.ObjectTypeBroadcast {

				}

				// Send the object out on the network.
				_, err = s.bmd.SendObject(obj)
				if err != nil {
					serverLog.Errorf("Failed to send object: %v", err)
				}

			} else if err != store.ErrNotFound {
				serverLog.Criticalf("Peek on PowQueue failed: %v", err)
			}

			// The only allowed error is store.ErrNotFound, which means that
			// there is nothing to process.
		}
	}
}

// savePeriodically periodically saves data in memory to the disk. This is to
// ensure that everything isn't lost in case of power failure/sudden shutdown.
func (s *server) savePeriodically() {
	defer s.wg.Done()

	t := time.NewTicker(saveInterval)
	for {
		select {
		case <-s.quit:
			return
		case <-t.C:
			s.saveData()
		}
	}
}

// saveData saves any data in memory to disk. This includes writing the keyfile
// and counter values to the store.
func (s *server) saveData() {
	saveKeyfile(s.keymgr, cfg.keyfilePass, cfg.keyfilePath)

	// Save counter values to store.
	err := s.store.SetCounter(wire.ObjectTypeMsg,
		atomic.LoadUint64(&s.msgCounter))
	if err != nil {
		serverLog.Critical("Failed to save message counter:", err)
	}

	err = s.store.SetCounter(wire.ObjectTypeBroadcast,
		atomic.LoadUint64(&s.broadcastCounter))
	if err != nil {
		serverLog.Critical("Failed to save broadcast counter:", err)
	}

	err = s.store.SetCounter(wire.ObjectTypeGetPubKey,
		atomic.LoadUint64(&s.getpubkeyCounter))
	if err != nil {
		serverLog.Critical("Failed to save getpubkey counter:", err)
	}
}

// getOrRequestPublicIdentity retrieves the needed public identity from bmd
// or sends a getpubkey request if it doesn't exist in its store. If both return
// types are nil, it means a getpubkey request has been queued.
func (s *server) getOrRequestPublicIdentity(address string) (*identity.Public, error) {
	id, err := s.bmd.GetIdentity(address)
	if err == nil {
		return id, nil
	} else if err != rpc.ErrIdentityNotFound {
		return nil, err
	}

	addr, err := bmutil.DecodeAddress(address)
	if err != nil {
		return nil, fmt.Errorf("Failed to decode address: %v", err)
	}

	// We don't have the identity so craft a getpubkey request.
	var tag wire.ShaHash
	copy(tag[:], addr.Tag())

	msg := wire.NewMsgGetPubKey(0, time.Now().Add(getpubkeyExpiry), addr.Version,
		addr.Stream, (*wire.RipeHash)(&addr.Ripe), &tag)

	// Enqueue the request for proof-of-work.
	b := wire.EncodeMessage(msg)[8:] // exclude nonce
	target := pow.CalculateTarget(uint64(len(b)),
		uint64(msg.ExpiresTime.Sub(time.Now()).Seconds()),
		pow.DefaultNonceTrialsPerByte, pow.DefaultExtraBytes)

	_, err = s.store.PowQueue.Enqueue(target, b)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

// Stop shutdowns all the servers.
func (s *server) Stop() {
	// Make sure this only happens once.
	if atomic.AddInt32(&s.shutdown, 1) != 1 {
		return
	}

	s.saveData()

	s.bmd.Stop()
	close(s.quit)
}

// WaitForShutdown waits until all the servers and RPC client have stopped
// before returning.
func (s *server) WaitForShutdown() {
	s.wg.Wait()
	s.bmd.WaitForShutdown()
}
