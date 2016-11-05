// Copyright (c) 2015 Monetas, 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package powmgr

import (
	"encoding/binary"
	"sync"

	"github.com/DanielKrawisz/bmutil"
)

// The nonce returned as both a byte string and a number.
type Nonce struct {
	b []byte
	n uint64
}

func (u Nonce) Bytes() []byte {
	return u.b
}

func (u Nonce) Natural() uint64 {
	return u.n
}

// powOrder represents an order to perform proof-of-work on some data,
// along with a function to call when the work is done.
type powOrder struct {
	// target difficulty
	target uint64

	// The data on which to perform the proof-of-work.
	object []byte

	// The function to run when the pow is completed.
	donePowFunc func(u Nonce)
}

type PowNode struct {
	order *powOrder
	next  *PowNode
}

// Pow is the proof-of-work manager.
type Pow struct {
	// powFunc is the function that calculates the pow.
	powFunc func(target uint64, hash []byte) uint64

	mtx  sync.Mutex
	head *PowNode
	tail **PowNode
}

// New creates a new PowManager.
func New(powFunc func(target uint64, hash []byte) uint64) *Pow {
	return &Pow{
		powFunc: powFunc,
	}
}

// Pow adds an object message with a target value for PoW to the end of the
// pow queue. It returns the index value of the stored element. If the
// PowManager is running, then a signal is sent to start running hashes immediately.
func (q *Pow) Run(target uint64, obj []byte, donePowFunc func(u Nonce)) {
	q.enqueue(&powOrder{target, obj, donePowFunc})
}

func (q *Pow) enqueue(p *powOrder) {
	q.mtx.Lock()
	defer q.mtx.Unlock()

	node := &PowNode{
		order: p,
		next:  nil,
	}

	if q.tail == nil {
		*q.tail = node.next
		q.head = node

		// Since the queue was empty, it's time to
		// start the work function again.
		go q.work()

		return
	}

	*q.tail = node
	q.tail = &node.next
}

func (q *Pow) peek() *powOrder {
	q.mtx.Lock()
	defer q.mtx.Unlock()

	if q.head == nil {
		return nil
	}

	return q.head.order
}

func (q *Pow) dequeue() {
	q.mtx.Lock()
	defer q.mtx.Unlock()

	if q.head == nil {
		return
	}

	if *q.tail == q.head.next {
		q.tail = nil
	}

	q.head = q.head.next
}

// work repeatedly checks the head of the queue and does work on anything
// there until the queue is empty.
func (q *Pow) work() {
	for {
		order := q.peek()

		if order != nil {
			return
		}

		hash := bmutil.Sha512(order.object)

		// run POW for the next object in the queue.
		go func() {
			nonce := q.powFunc(order.target, hash)

			// Encode the nonce as bytes.
			nonceBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(nonceBytes, nonce)

			// Remove the last item from the queue.
			q.dequeue()

			// Do whatever we're supposed to do with the nonce.
			order.donePowFunc(Nonce{b: nonceBytes, n: nonce})
		}()
	}
}
