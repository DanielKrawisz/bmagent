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

	"github.com/jordwest/imap-server"
	"github.com/DanielKrawisz/bmagent/email"
	"github.com/DanielKrawisz/bmagent/keymgr"
	"github.com/DanielKrawisz/bmagent/powmgr"
	"github.com/DanielKrawisz/bmagent/rpc"
	"github.com/DanielKrawisz/bmagent/store"
	"github.com/DanielKrawisz/bmutil"
	"github.com/DanielKrawisz/bmutil/cipher"
	"github.com/DanielKrawisz/bmutil/identity"
	"github.com/DanielKrawisz/bmutil/pow"
	"github.com/DanielKrawisz/bmutil/wire"
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
	smtp             *email.SMTPServer
	smtpListeners    []net.Listener
	imap             *imap.Server
	imapUser         map[uint32]*email.User
	imapListeners    []net.Listener
	quit             chan struct{}
	wg               sync.WaitGroup
}

// newServer initializes a new instance of server.
func newServer(rpcc *rpc.ClientConfig, user *User, s *store.Store, pk *store.PKRequests) (*server, error) {

	srvr := &server{
		users:         make(map[uint32]*User),
		store:         s,
		pk:            pk, 
		smtpListeners: make([]net.Listener, 0, len(cfg.SMTPListeners)),
		imapListeners: make([]net.Listener, 0, len(cfg.IMAPListeners)),
		quit:          make(chan struct{}),
		imapUser:      make(map[uint32]*email.User), 
	}
	
	var err error
	srvr.bmd, err = rpc.NewClient(rpcc, srvr.newMessage, srvr.newBroadcast, srvr.newGetpubkey)
	if err != nil {
		log.Errorf("Cannot create bmd server RPC client: %v", err)
		return nil, err
	}
	
	// TODO allow for more than one user. Right now there is just 1 user.
	srvr.users[1] = user
	userData, err := s.GetUser(user.Username)
	if err != nil {
		return nil, err
	}
	
	so := &serverOps{
		pubIDs: make(map[string]*identity.Public),
		id: 1, 
		user: user, 
		server: srvr,
		data: userData, 
	}
		
	// Create an email.User from the store.
	imapUser, err := email.NewUser(user.Username, so, user.Keys)
	if cfg.GenKeys > 0 {
		imapUser.GenerateKeys(uint16(cfg.GenKeys));
	}
	if err != nil {
		return nil, err
	}
	srvr.imapUser[1] = imapUser

	// Setup SMTP and IMAP servers.
	srvr.smtp = email.NewSMTPServer(&email.SMTPConfig{
		RequireTLS: !cfg.DisableServerTLS,
		Username:   cfg.Username,
		Password:   cfg.Password,
	}, imapUser)
	srvr.imap = imap.NewServer(email.NewBitmessageStore(imapUser, &email.IMAPConfig{
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
	// Whether the message was received from a channel.
	var ofChan bool
	
	var id uint32

	// Try decrypting with all available identities.
	for uid, user := range s.users {
		err = user.Keys.ForEach(func(id *keymgr.PrivateID) error {
			if cipher.TryDecryptAndVerifyMsg(msg, &id.Private) == nil {
				address = id.Address()
				ofChan = id.IsChan
				return errors.New("decryption successful")
			}
			return nil
		})
	
		if err != nil {
			id = uid
			// Decryption successful.
			break
		}
	}
	
	if err == nil {
		// Decryption unsuccessful.
		return
	}

	// Decryption was successful. Add message to store.

	// TODO Store public key of the sender in bmagent

	// Read message.
	bmsg, err := email.MsgRead(msg, address, ofChan)
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
	if len(msg.Ack) < wire.MessageHeaderSize || ofChan {
		return
	}

	// Send out ack if necessary.
	ack := &wire.MsgObject{}
	err = ack.Decode(bytes.NewReader(msg.Ack[wire.MessageHeaderSize:]))
	if err != nil { // Can't send invalid Ack.
		return
	}
	_, err = s.bmd.SendObject(msg.Ack[wire.MessageHeaderSize:])
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

	for _, user := range s.store.Users {

		err = user.BroadcastAddresses.ForEach(func(addr *bmutil.Address) error {
			if cipher.TryDecryptAndVerifyBroadcast(msg, addr) == nil {
				fromAddress, _ = addr.Encode()
				return errors.New("Broadcast decryption succeeded.")
			}
			return nil
		})
		if err == nil { // Broadcast decryption failed.
			return
		}
		
	} 
	
	// Read message.
	bmsg, err := email.BroadcastRead(msg)
	if err != nil {
		log.Errorf("Failed to decode message #%d: %v", counter, err)
		return
	}
	
	rpccLog.Trace("Bitmessage broadcast received from " + bmsg.From + " to " + bmsg.To)

	err = s.imapUser[1].DeliverFromBMNet(bmsg)
	if err != nil {
		log.Errorf("Failed to save message #%d: %v", counter, err)
		return
	}
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
	for _, user := range s.users {
		err = user.Keys.ForEach(func(id *keymgr.PrivateID) error {
			if id.Disabled || id.IsChan { // We don't care about these.
				return nil
			}
	
			var cond bool // condition to satisfy
			switch msg.Version {
			case wire.SimplePubKeyVersion, wire.ExtendedPubKeyVersion:
				cond = bytes.Equal(id.Private.Address.Ripe[:], msg.Ripe[:])
			case wire.TagGetPubKeyVersion:
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
	b := wire.EncodeMessage(pkMsg)[8:] // exclude nonce
	target := pow.CalculateTarget(uint64(len(b)),
		uint64(pkMsg.ExpiresTime.Sub(time.Now()).Seconds()),
		pow.DefaultNonceTrialsPerByte, pow.DefaultExtraBytes)

	s.pow.Run(target, b, nil)
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

func (s *server) Send(obj []byte) error {	// Send the object out on the network.
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

	msg := wire.NewMsgGetPubKey(0, time.Now().Add(defaultGetpubkeyExpiry),
		addr.Version, addr.Stream, (*wire.RipeHash)(&addr.Ripe), &tag)

	// Enqueue the request for proof-of-work.
	b := wire.EncodeMessage(msg)[8:] // exclude nonce
	target := pow.CalculateTarget(uint64(len(b)),
		uint64(msg.ExpiresTime.Sub(time.Now()).Seconds()),
		pow.DefaultNonceTrialsPerByte, pow.DefaultExtraBytes)

	s.pow.Run(target, b, func(nonce powmgr.Nonce) {
		s.Send(append(nonce.Bytes(), b...))
	})

	// Store a record of the public key request.
	count, err := s.pk.New(address)
	if err != nil {
		return nil, err
	}

	serverLog.Tracef("getOrRequestPublicIdentity: Requested address %s %d time(s).",
		address, count)
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
