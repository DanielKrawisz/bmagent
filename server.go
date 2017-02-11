// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/aes"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DanielKrawisz/bmagent/keymgr/keys"
	"github.com/DanielKrawisz/bmagent/powmgr"
	"github.com/DanielKrawisz/bmagent/rpc"
	"github.com/DanielKrawisz/bmagent/store"
	"github.com/DanielKrawisz/bmagent/user"
	"github.com/DanielKrawisz/bmagent/user/email"
	"github.com/DanielKrawisz/bmutil"
	"github.com/DanielKrawisz/bmutil/cipher"
	"github.com/DanielKrawisz/bmutil/identity"
	"github.com/DanielKrawisz/bmutil/pow"
	"github.com/DanielKrawisz/bmutil/wire"
	"github.com/DanielKrawisz/bmutil/wire/obj"
	"github.com/jordwest/imap-server"
)

const (
	// pkCheckerInterval is the interval after which bmclient should query bmd
	// for retrieving any new public identities (that we queried for before).
	pkCheckerInterval = time.Minute * 2

	// powCheckerInterval is the interval after with bmclient should check the
	// proof-of-work queue and process next item.
	powCheckerInterval = time.Second * 5

	// saveInterval is the interval after which data in memory should be saved
	// to disk.
	saveInterval = time.Minute * 5
)

// server struct manages everything that a running instance of bmclient
// comprises of and would need.
type server struct {
	bmd              *rpc.Client
	users            map[uint32]*User
	store            *store.Store
	pk               *store.PKRequests
	pow              *powmgr.Pow
	started          int32
	shutdown         int32
	msgCounter       uint64
	broadcastCounter uint64
	getpubkeyCounter uint64
	smtp             *user.SMTPServer
	smtpListeners    []net.Listener
	imap             *imap.Server
	imapUser         map[uint32]*user.User
	imapListeners    []net.Listener
	quit             chan struct{}
	wg               sync.WaitGroup
}

// newServer initializes a new instance of server.
func newServer(rpcc *rpc.ClientConfig, u *User, s *store.Store, pk *store.PKRequests) (*server, error) {

	srvr := &server{
		users:         make(map[uint32]*User),
		store:         s,
		pk:            pk,
		smtpListeners: make([]net.Listener, 0, len(cfg.SMTPListeners)),
		imapListeners: make([]net.Listener, 0, len(cfg.IMAPListeners)),
		quit:          make(chan struct{}),
		imapUser:      make(map[uint32]*user.User),
	}

	var err error
	srvr.bmd, err = rpc.NewClient(rpcc, srvr.newMessage, srvr.newBroadcast, srvr.newGetpubkey)
	if err != nil {
		log.Errorf("Cannot create bmd server RPC client: %v", err)
		return nil, err
	}

	// TODO allow for more than one user. Right now there is just 1 user.
	srvr.users[1] = u
	userData, err := s.GetUser(u.Username)
	if err != nil {
		return nil, err
	}

	so := &serverOps{
		pubIDs: make(map[string]*identity.Public),
		id:     1,
		user:   u,
		server: srvr,
		data:   userData,
	}

	// Create an user.User from the store.
	imapUser, err := user.NewUser(u.Username, u.Keys, ObjectExpiration, so)
	if err != nil {
		return nil, err
	}
	srvr.imapUser[1] = imapUser

	// Setup SMTP and IMAP servers.
	srvr.smtp = user.NewSMTPServer(&email.SMTPConfig{
		RequireTLS: !cfg.DisableServerTLS,
		Username:   cfg.Username,
		Password:   cfg.Password,
	}, imapUser)
	srvr.imap = imap.NewServer(user.NewBitmessageStore(imapUser, &email.IMAPConfig{
		RequireTLS: !cfg.DisableServerTLS,
		Username:   cfg.Username,
		Password:   cfg.Password,
	}))

	// Setup tracer for IMAP.
	// srvr.imap.Transcript = os.Stderr

	// Setup IMAP listeners.
	for _, laddr := range cfg.IMAPListeners {
		l, err := net.Listen("tcp", laddr)
		if err != nil {
			return nil, imapLog.Criticalf("Failed to listen on %s: %v", l, err)
		}
		srvr.imapListeners = append(srvr.imapListeners, l)
	}

	// Setup SMTP listeners.
	for _, laddr := range cfg.SMTPListeners {
		l, err := net.Listen("tcp", laddr)
		if err != nil {
			return nil, smtpLog.Criticalf("Failed to listen on %s: %v", l, err)
		}
		srvr.smtpListeners = append(srvr.smtpListeners, l)
	}

	// Load counter values from store.
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

	// Setup pow manager.
	srvr.pow = powmgr.New(cfg.powHandler)

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
	for _, l := range s.imapListeners {
		imapLog.Infof("Listening on %s", l.Addr())
		go s.imap.Serve(l)
	}

	// Start SMTP server.
	for _, l := range s.smtpListeners {
		smtpLog.Infof("Listening on %s", l.Addr())
		go s.smtp.Serve(l)
	}

	// TODO Start RPC server.

	// Start public key request handler.
	serverLog.Info("Starting public key request handler.")
	s.wg.Add(1)
	go s.pkRequestHandler()

	// Start saving data periodically.
	s.wg.Add(1)
	go s.savePeriodically()
}

var errSuccessCode = errors.New("decryption successful")

// newMessage is called when a new message is received by the RPC client.
// Messages are guaranteed to be received in ascending order of counter value.
func (s *server) newMessage(counter uint64, object []byte) {
	// Store counter value.
	atomic.StoreUint64(&s.msgCounter, counter)

	msg := &obj.Message{}
	err := msg.Decode(bytes.NewReader(object))
	if err != nil {
		serverLog.Errorf("Failed to decode message #%d from bytes: %v", counter,
			err)
		return // Ignore message.
	}

	// Is this an ack message we are expecting?
	if len(object) == cipher.AckLength {
		for _, u := range s.imapUser {
			err = u.DeliverAckReply(wire.InventoryHash(object))
			if err != user.ErrUnrecognizedAck {
				if err != nil {
					serverLog.Errorf("Error returned receiving ack: %v", err)
				}
				return
			}
		}

		return
	}

	// Check if message is smaller than expected.
	// IV + Curve params/X/Y + 1 block + HMAC-256
	if len(msg.Encrypted) <= aes.BlockSize+70+aes.BlockSize+sha256.Size {
		return
	}

	// Contains the address of the identity used to decrypt the message.
	var address string
	// Whether the message was received from a channel.
	var ofChan bool

	var id uint32
	var message *cipher.Message

	// Try decrypting with all available identities.
	for uid, user := range s.users {
		err = user.Keys.ForEach(func(id *keys.PrivateID) error {
			var decryptErr error
			message, decryptErr = cipher.TryDecryptAndVerifyMessage(msg, &id.Private)
			if decryptErr == nil {
				address = id.Address()
				ofChan = id.IsChan
				//id = uid
				return errSuccessCode
			}
			return nil
		})

		if err != nil {
			id = uid
			// Decryption successful.
			break
		}
	}

	if message == nil {
		// Decryption unsuccessful.
		return
	}

	// Decryption was successful.

	// TODO Store public key of the sender in bmagent

	// Read message.
	bmsg, err := email.MsgRead(message, address, ofChan)
	if err != nil {
		log.Errorf("Failed to decode message #%d: %v", counter, err)
		return
	}

	rpccLog.Trace("Bitmessage received from " + bmsg.From + " to " + bmsg.To)

	err = s.imapUser[id].DeliverFromBMNet(bmsg)
	if err != nil {
		log.Errorf("Failed to save message #%d: %v", counter, err)
		return
	}

	// Check if length of Ack is correct and message isn't from a channel.
	ack := message.Ack()
	if ack == nil || len(ack) < wire.MessageHeaderSize || ofChan {
		log.Debugf("No ack found #%d", counter)
		return
	} else if len(ack) < wire.MessageHeaderSize || ofChan {
		log.Errorf("Ack too big #%d.", counter)
		return
	}

	// Send ack message if it exists.
	ackObj := ack[wire.MessageHeaderSize:]
	err = (&wire.MsgObject{}).Decode(bytes.NewReader(ackObj))
	if err != nil { // Can't send invalid Ack.
		log.Errorf("Failed to decode ack #%d: %v", counter, err)
		return
	}
	_, err = s.bmd.SendObject(ack)
	if err != nil {
		log.Infof("Failed to send ack for message #%d: %v", counter, err)
		return
	}

}

// newBroadcast is called when a new broadcast is received by the RPC client.
// Broadcasts are guaranteed to be received in ascending order of counter value.
func (s *server) newBroadcast(counter uint64, object []byte) {
	// Store counter value.
	atomic.StoreUint64(&s.broadcastCounter, counter)

	msg, err := obj.DecodeBroadcast(object)
	if err != nil {
		serverLog.Errorf("Failed to decode broadcast #%d from bytes: %v",
			counter, err)
		return // Ignore message.
	}

	// Check if broadcast is smaller than expected.
	// IV + Curve params/X/Y + 1 block + HMAC-256
	if len(msg.Encrypted()) <= aes.BlockSize+70+aes.BlockSize+sha256.Size {
		return
	}

	for _, user := range s.store.Users {

		err := user.BroadcastAddresses.ForEach(func(addr *bmutil.Address) error {
			var fromAddress string
			broadcast, err := cipher.TryDecryptAndVerifyBroadcast(msg, addr)
			if err == nil {
				fromAddress, _ = addr.Encode()
			}

			// Read message.
			bmsg, err := email.BroadcastRead(broadcast)
			if err != nil {
				return fmt.Errorf("Failed to decode message #%d: %v", counter, err)
			}

			rpccLog.Trace("Bitmessage broadcast received from " + bmsg.From + " to " + bmsg.To)

			err = s.imapUser[1].DeliverFromBMNet(bmsg)
			if err != nil {
				return fmt.Errorf("Failed to save message #%d: %v", counter, err)
			}
			serverLog.Infof("Got new broadcast from %s", fromAddress)

			return errSuccessCode
		})

		if err != nil && err != errSuccessCode {
			log.Error(err)
		}
	}
}

// newGetpubkey is called when a new getpubkey is received by the RPC client.
// Getpubkey requests are guaranteed to be received in ascending order of
// counter value.
func (s *server) newGetpubkey(counter uint64, object []byte) {
	// Store counter value.
	atomic.StoreUint64(&s.getpubkeyCounter, counter)

	msg := &obj.GetPubKey{}
	err := msg.Decode(bytes.NewReader(object))
	if err != nil {
		serverLog.Errorf("Failed to decode getpubkey #%d from bytes: %v",
			counter, err)
		return // Ignore message.
	}

	var privID *keys.PrivateID
	header := msg.Header()

	// Check if the getpubkey request corresponds to any of our identities.
	for _, user := range s.users {
		err = user.Keys.ForEach(func(id *keys.PrivateID) error {
			if id.Disabled || id.IsChan { // We don't care about these.
				return nil
			}

			var cond bool // condition to satisfy
			switch header.Version {
			case obj.SimplePubKeyVersion, obj.ExtendedPubKeyVersion:
				cond = bytes.Equal(id.Private.Address.Ripe[:], msg.Ripe[:])
			case obj.TagGetPubKeyVersion:
				cond = bytes.Equal(id.Private.Address.Tag(), msg.Tag[:])
			}
			if cond {
				privID = id
				return errors.New("We have a match.")
			}
			return nil
		})
		if err != nil {
			break
		}
	}
	if err == nil {
		return
	}

	addr := privID.Address()
	serverLog.Infof("Received a getpubkey request for %s, sending out the pubkey.", addr)

	// Generate a pubkey message.
	pkMsg, err := cipher.GeneratePubKey(&privID.Private, defaultPubkeyExpiry)
	if err != nil {
		serverLog.Errorf("Failed to generate pubkey for %s: %v", addr, err)
		return
	}

	// Add it to pow queue.
	pkObj := pkMsg.Object()
	pkHeader := pkObj.Header()
	b := wire.Encode(pkObj)[8:] // exclude nonce
	target := pow.CalculateTarget(uint64(len(b)),
		uint64(pkHeader.Expiration().Sub(time.Now()).Seconds()),
		pow.DefaultData)

	s.pow.Run(target, b, func(nonce pow.Nonce) {
		s.Send(append(nonce.Bytes(), b...))
	})
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
			var mtx sync.Mutex // Protect the following map
			addresses := make(map[string]*identity.Public)
			var wg sync.WaitGroup

			// Go through our store and check if server has any new public
			// identity.
			s.pk.ForEach(func(address string, reqCount uint32,
				lastReqTime time.Time) error {

				serverLog.Debugf("Checking whether we have public key for %s.",
					address)
				wg.Add(1)

				// Run requests in parellel because they're I/O dependent.
				go func(addr string) {
					defer wg.Done()

					public, err := s.bmd.GetIdentity(addr)
					if err == rpc.ErrIdentityNotFound {
						serverLog.Debug("identity not found for ", addr)
						// TODO Check whether lastReqTime has exceeded a set
						// constant. If it has, send a new request.

						// TODO Check whether reqCount has exceeded a set
						// constant. If it has, delete the message from queue
						// and generate a bounce message.
						return
					} else if err != nil {
						rpccLog.Errorf("GetIdentity(%s) gave unexpected error %v",
							addr, err)
						return
					}
					mtx.Lock()
					addresses[addr] = public
					mtx.Unlock()
				}(address)

				return nil
			})
			wg.Wait()

			for address, public := range addresses {
				serverLog.Debugf("Received pubkey for %s. Processing pending messages.",
					address)

				// Process pending messages with this public identity and add
				// them to pow queue.
				for _, user := range s.imapUser {
					err := user.DeliverPublicKey(address, public)
					if err != nil {
						serverLog.Error("DeliverPublicKey failed: ", err)
					}
				}

				// Now that we have the public identities, remove them from the
				// PK request store.
				err := s.pk.Remove(address)
				if err != nil {
					serverLog.Critical("Failed to remove address from public"+
						" key request store: ", err)
				}
			}
		}
	}
}

// Send the object out on the network.
func (s *server) Send(obj []byte) error {
	_, err := s.bmd.SendObject(obj)
	if err != nil {
		serverLog.Error("Failed to send object:", err)
		return err
	}

	return nil
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
	for _, user := range s.users {
		user.SaveKeyfile()
	}

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
// or sends a getpubkey request if it doesn't exist in its database. If both
// return types are nil, it means a getpubkey request has been queued.
func (s *server) getOrRequestPublicIdentity(user uint32, address string) (*identity.Public, error) {
	serverLog.Debug("getOrRequestPublicIdentity called for ", address)
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

	serverLog.Debug("getOrRequestPublicIdentity: address not found, send pubkey request.")
	// We don't have the identity so craft a getpubkey request.
	var tag wire.ShaHash
	copy(tag[:], addr.Tag())

	msg := obj.NewGetPubKey(0, time.Now().Add(defaultGetpubkeyExpiry),
		addr.Version, addr.Stream, (*wire.RipeHash)(&addr.Ripe), &tag)

	// Enqueue the request for proof-of-work.
	b := wire.Encode(msg)[8:] // exclude nonce
	target := pow.CalculateTarget(uint64(len(b)),
		uint64(msg.Header().Expiration().Sub(time.Now()).Seconds()),
		pow.DefaultData)

	s.pow.Run(target, b, func(nonce pow.Nonce) {
		err := s.Send(append(nonce.Bytes(), b...))
		if err != nil {
			serverLog.Error("Could not run pow: ", err)
		}

		// Store a record of the public key request.
		count, _ := s.pk.New(address)
		serverLog.Tracef("getOrRequestPublicIdentity: Requested address %s %d time(s).",
			address, count)
	})

	return nil, nil
}

// Stop shutdowns all the servers.
func (s *server) Stop() {
	// Make sure this only happens once.
	if atomic.AddInt32(&s.shutdown, 1) != 1 {
		return
	}

	// Save any data in memory.
	s.saveData()

	// Close all SMTP listeners.
	for _, l := range s.smtpListeners {
		l.Close()
	}

	// Close all IMAP listeners.
	for _, l := range s.imapListeners {
		l.Close()
	}
	s.imapUser = nil // Prevent pointer cycle.

	s.bmd.Stop()
	close(s.quit)
}

// WaitForShutdown waits until all the servers and RPC client have stopped
// before returning.
func (s *server) WaitForShutdown() {
	s.wg.Wait()
	s.bmd.WaitForShutdown()
}
