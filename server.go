// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/monetas/bmclient/keymgr"
	"github.com/monetas/bmclient/store"
	"github.com/monetas/bmutil/pow"
)

const (
	// pkCheckerInterval is the interval after which bmclient should query bmd
	// for retrieving any new public identities (that we queried for before).
	pkCheckerInterval = time.Minute * 5

	// powCheckerInterval is the interval after with bmclient should check the
	// proof-of-work queue and process next item.
	powCheckerInterval = time.Second * 10
)

// server struct manages everything that a running instance of bmclient
// comprises of and would need.
type server struct {
	bmd      *rpcClient
	kmgr     *keymgr.Manager
	store    *store.Store
	started  int32
	shutdown int32
	quit     chan struct{}
	wg       sync.WaitGroup
}

// newServer initializes a new instance of server.
func newServer(bmd *rpcClient, kmgr *keymgr.Manager,
	s *store.Store) (*server, error) {
	return &server{
		bmd:   bmd,
		kmgr:  kmgr,
		store: s,
	}, nil
}

// Start starts all the servers one by one and returns an error if any fails.
func (s *server) Start() {
	// Already started?
	if atomic.AddInt32(&s.started, 1) != 1 {
		return
	}

	// Start IMAP server.

	// Start SMTP server.

	// Start RPC server.

	// Start public key request handler.
	go s.pkRequestHandler()

	// Start proof of work handler.
	go s.powHandler()
}

// pkRequestHandler manages the pubkey request store. It periodically checks
// with bmd whether the requested identities have been received. If they have,
// it removes them from the pubkey request store and processes messages that
// need that identity.
func (s *server) pkRequestHandler() {
	s.wg.Add(1)

	t := time.NewTicker(pkCheckerInterval)

outer:
	for {
		select {
		case <-s.quit:
			break outer
		case <-t.C:
			// Go through our store and check if server has any new public
			// identity.
			s.store.PubkeyRequests.ForEach(func(address string, addTime time.Time) {
				_, err := s.bmd.GetIdentity(address)
				if err != nil {
					// bmd still doesn't have the public key.
					// TODO more concrete error checking; specify exact error in README_RPC.md
					if !strings.Contains(err.Error(), "not found") {
						rpccLog.Errorf("Calling GetIdentity on bmd gave "+
							"unexpected error %v", err)
						return

					}

					// TODO check whether addTime has exceeded a set constant.
					// If it has, delete the message from queue and generate a
					// bounce message.

					return
				}
				// Now that we have the public identity, remove it from the
				// PK request store.
				err = s.store.PubkeyRequests.Remove(address)
				if err != nil {
					serverLog.Criticalf("Failed to remove address from public key"+
						" request store: %v", err)
				}

				// TODO process pending messages with this public identity and
				// add them to pow queue.

			})
		}
	}

	s.wg.Done()
}

// powHandler manages the proof-of-work queue. It makes sure that only one
// object is processed at a time. After doing POW, it sends the object out on
// the network.
func (s *server) powHandler() {
	s.wg.Add(1)

	t := time.NewTicker(powCheckerInterval)

outer:
	for {
		select {
		case <-s.quit:
			break outer
		case <-t.C:
			target, hash, err := s.store.PowQueue.Peek()
			if err == nil { // We have something to process
				// TODO add config option for choosing POW implementation and
				// number of CPUs to use.
				_ = pow.DoParallel(target, hash[:], 2)

				// Since we have the required nonce value and have processed
				// the pending message, remove it from the queue.
				_, _, err = s.store.PowQueue.Dequeue()
				if err != nil {
					serverLog.Criticalf("Dequeue on PowQueue failed: %v", err)
					continue
				}

				// TODO process messages in pending queue that needed POW
				// and queue them for sending out on the network.

			} else if err != store.ErrNotFound {
				serverLog.Criticalf("Peek on PowQueue failed: %v", err)
			}

			// The only allowed error is store.ErrNotFound, which means that
			// there is nothing to process.
		}
	}
	s.wg.Done()
}

// Stop shutdowns all the servers.
func (s *server) Stop() {
	// Make sure this only happens once.
	if atomic.AddInt32(&s.shutdown, 1) != 1 {
		return
	}

	close(s.quit)
	s.bmd.Close()
}

// WaitForShutdown waits until all the servers have stopped before returning.
func (s *server) WaitForShutdown() {
	s.wg.Wait()
}
