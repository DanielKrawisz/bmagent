// Copyright (c) 2015 Monetas, 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package powmgr

import (
	"sync"

	"github.com/DanielKrawisz/bmutil/hash"
	"github.com/DanielKrawisz/bmutil/pow"
)

// powOrder represents an order to perform proof-of-work on some data,
// along with a function to call when the work is done.
type powOrder struct {
	// target difficulty
	target pow.Target

	// The data on which to perform the proof-of-work.
	object []byte

	// The function to run when the pow is completed.
	donePowFunc func(u pow.Nonce)
}

type powNode struct {
	order *powOrder
	next  *powNode
}

// Pow is the proof-of-work manager.
type Pow struct {
	// powFunc is the function that calculates the pow.
	powFunc func(target pow.Target, hash []byte) pow.Nonce

	mtx  sync.Mutex
	head *powNode
	tail **powNode
}

// New creates a new PowManager.
func New(powFunc func(target pow.Target, hash []byte) pow.Nonce) *Pow {
	return &Pow{
		powFunc: powFunc,
	}
}

// Run adds an object message with a target value for PoW to the end of the
// pow queue. It returns the index value of the stored element. If the
// PowManager is running, then a signal is sent to start running hashes immediately.
func (q *Pow) Run(target pow.Target, obj []byte, donePowFunc func(u pow.Nonce)) {
	q.enqueue(&powOrder{target, obj, donePowFunc})
}

func (q *Pow) enqueue(p *powOrder) {
	q.mtx.Lock()
	defer q.mtx.Unlock()

	node := &powNode{
		order: p,
		next:  nil,
	}

	if q.head == nil {
		q.tail = &node.next
		q.head = node

		// Since the queue was empty, it's time to
		// start the work function again.
		go q.work()

		return
	}

	*q.tail = node
	q.tail = &node.next
}

func (q *Pow) dequeue() *powOrder {
	q.mtx.Lock()
	defer q.mtx.Unlock()

	if q.head == nil {
		return nil
	}

	p := q.head.order

	q.head = q.head.next

	return p
}

// work repeatedly checks the head of the queue and does work on anything
// there until the queue is empty.
func (q *Pow) work() {
	for {
		order := q.dequeue()

		if order == nil {
			return
		}

		hash := hash.Sha512(order.object)

		// run POW for the next object in the queue.
		n := q.powFunc(order.target, hash)

		// Remove the last item from the queue.
		q.dequeue()

		// Do whatever we're supposed to do with the nonce.
		go order.donePowFunc(n)
	}
}
