// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package powmgr

import (
	"encoding/binary"
	"sync"

	"github.com/monetas/bmclient/store"
)

// PowManager manages the calculation of proof-of-work in the pow queue.
// While it is running, when it receives a message that a new item has been
// added to the pow queue, it goes down the queue and runs the pow for every
// item in the queue, and then sends the completed item to the server.
type PowManager struct {
	powQueue *store.PowQueue
	// newPowRequest signals that a new pow request has been enqueued.
	newPowChan chan struct{}
	// quitChan signals the pow handler to quit.
	quitChan chan struct{}
}

// New creates a new PowManager.
func New(pq *store.PowQueue) *PowManager {
	return &PowManager{
		powQueue: pq,
		quitChan: make(chan struct{}),
	}
}

// RunPow adds an object message with a target value for PoW to the end of the
// pow queue. It returns the index value of the stored element. If the
// PowManager is running, then a signal is sent to start running hashes immediately.
func (pm *PowManager) RunPow(target uint64, obj []byte) (uint64, error) {
	index, err := pm.powQueue.Enqueue(target, obj)
	if err != nil {
		return 0, err
	}

	// Signal to start processing the pow if the queue is running.
	if pm.newPowChan != nil {
		pm.newPowChan <- struct{}{}
	}

	return index, nil
}

// PowHandler manages the proof-of-work queue. It makes sure that only one
// object is processed at a time. After doing POW, it returns the object
// to the server.
// parameters:
//  * donePowFunc is used to signal that a nonce has been calculated.
//  * powFunc is the function that calculates the pow.
//  * wg is an optional wait group that is called when the function ends.
func (pm *PowManager) PowHandler(
	donePowFunc func(index uint64, obj []byte),
	powFunc func(target uint64, hash []byte) uint64,
	wg *sync.WaitGroup) {

	if wg != nil {
		defer wg.Done()
	}

	// If the PowHandler is awake, then it does not respond to signals
	// from newPowChan and just keeps processing down the queue until the
	// queue is empty. Then it goes to sleep.
	awake := true

	donePowChan := make(chan uint64)
	pm.newPowChan = make(chan struct{})

	// calculatePow handles the pow calculation for a single object.
	// Eventually, this might be upgraded so that it could be interrupted
	// in the middle of a calculation.
	calculatePow := func(target uint64, hash []byte) {
		donePowChan <- powFunc(target, hash)
	}

	// startNewPow peeks for the latest information from the queue and begins
	// processing it. It sets the queue asleep if none is found.
	startNewPow := func() {
		target, hash, err := pm.powQueue.PeekForPow()
		if err != nil {
			// The only allowed error is store.ErrNotFound, which means that
			// there is nothing to process.
			if err != store.ErrNotFound {
				log.Criticalf("Peek on PowQueue failed: %v", err)
			}

			awake = false
			return
		}

		// run POW for the next object in the queue.
		go calculatePow(target, hash)
	}

	startNewPow()

out:
	for {
		select {
		case <-pm.quitChan:
			break out
		case <-pm.newPowChan:
			// ignore if the pow handler is awake because it's already working.
			if !awake {
				awake = true
				startNewPow()
			}
		case nonce := <-donePowChan:
			// Since we have the required nonce value and have processed
			// the pending message, remove it from the queue.
			index, obj, err := pm.powQueue.Dequeue()
			if err != nil {
				log.Critical("Dequeue on PowQueue failed: ", err)
				continue
			}

			// Re-assemble message as bytes.
			nonceBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(nonceBytes, nonce)
			obj = append(nonceBytes, obj...)

			// Send the data to the server.
			donePowFunc(index, obj)

			startNewPow()
		}
	}
}

// Stop stops the pow queue.
func (pm *PowManager) Stop() {
	close(pm.quitChan)
}
