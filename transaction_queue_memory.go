// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"bytes"
	"container/list"
	"encoding/base64"
	"fmt"
	"sync"
)

// TransactionQueueMemory is an in-memory FIFO implementation of the TransactionQueue interface.
type TransactionQueueMemory struct {
	txMap        map[TransactionID]*list.Element
	txQueue      *list.List
	balanceCache *BalanceCache
	lock         sync.RWMutex
}

// NewTransactionQueueMemory returns a new NewTransactionQueueMemory instance.
func NewTransactionQueueMemory(ledger Ledger) *TransactionQueueMemory {
	// don't accept transactions that would leave an unspendable balance with this node
	var minBalance int64 = MIN_AMOUNT_CRUZBITS + MIN_FEE_CRUZBITS

	return &TransactionQueueMemory{
		txMap:        make(map[TransactionID]*list.Element),
		txQueue:      list.New(),
		balanceCache: NewBalanceCache(ledger, minBalance),
	}
}

// Add adds the transaction to the queue. Returns true if the transaction was added to the queue on this call.
func (t *TransactionQueueMemory) Add(id TransactionID, tx *Transaction) (bool, error) {
	t.lock.Lock()
	defer t.lock.Unlock()
	if _, ok := t.txMap[id]; ok {
		// already exists
		return false, nil
	}

	// check sender balance and update sender and receiver balances
	ok, err := t.balanceCache.Apply(tx)
	if err != nil {
		return false, err
	}
	if !ok {
		// insufficient sender balance
		return false, fmt.Errorf("Transaction %s sender %s has insufficient balance",
			id, base64.StdEncoding.EncodeToString(tx.From[:]))
	}

	// add to the back of the queue
	e := t.txQueue.PushBack(tx)
	t.txMap[id] = e
	return true, nil
}

// AddBatch adds a batch of transactions to the queue (a block has been disconnected.)
// "height" is the block chain height after this disconnection.
func (t *TransactionQueueMemory) AddBatch(ids []TransactionID, txs []*Transaction, height int64) error {
	t.lock.Lock()
	defer t.lock.Unlock()

	// add to front in reverse order.
	// we want formerly confirmed transactions to have the highest
	// priority for getting into the next block.
	for i := len(txs) - 1; i >= 0; i-- {
		if e, ok := t.txMap[ids[i]]; ok {
			// remove it from its current position
			t.txQueue.Remove(e)
		}
		e := t.txQueue.PushFront(txs[i])
		t.txMap[ids[i]] = e
	}

	// we don't want to invalidate anything based on maturity/expiration/balance yet.
	// if we're disconnecting a block we're going to be connecting some shortly.
	return nil
}

// RemoveBatch removes a batch of transactions from the queue (a block has been connected.)
// "height" is the block chain height after this connection.
// "more" indicates if more connections are coming.
func (t *TransactionQueueMemory) RemoveBatch(ids []TransactionID, height int64, more bool) error {
	t.lock.Lock()
	defer t.lock.Unlock()
	for _, id := range ids {
		e, ok := t.txMap[id]
		if !ok {
			// not in the queue
			continue
		}
		// remove it
		t.txQueue.Remove(e)
		delete(t.txMap, id)
	}

	if more {
		// we don't want to invalidate anything based on series/maturity/expiration/balance
		// until we're done connecting all of the blocks we intend to
		return nil
	}

	return t.reprocessQueue(height)
}

// Rebuild the balance cache and remove transactions now in violation
func (t *TransactionQueueMemory) reprocessQueue(height int64) error {
	// invalidate the cache
	t.balanceCache.Reset()

	// remove invalidated transactions from the queue
	tmpQueue := list.New()
	tmpQueue.PushBackList(t.txQueue)
	for e := tmpQueue.Front(); e != nil; e = e.Next() {
		tx := e.Value.(*Transaction)
		// check that the series would still be valid
		if !checkTransactionSeries(tx, height+1) ||
			// check maturity and expiration if included in the next block
			!tx.IsMature(height+1) || tx.IsExpired(height+1) ||
			// don't re-mine any now unconfirmed spam
			tx.Fee < MIN_FEE_CRUZBITS || tx.Amount < MIN_AMOUNT_CRUZBITS {
			// transaction has been invalidated. remove and continue
			id, err := tx.ID()
			if err != nil {
				return err
			}
			e := t.txMap[id]
			t.txQueue.Remove(e)
			delete(t.txMap, id)
			continue
		}

		// check balance
		ok, err := t.balanceCache.Apply(tx)
		if err != nil {
			return err
		}
		if !ok {
			// transaction has been invalidated. remove and continue
			id, err := tx.ID()
			if err != nil {
				return err
			}
			e := t.txMap[id]
			t.txQueue.Remove(e)
			delete(t.txMap, id)
			continue
		}
	}
	return nil
}

// Get returns transactions in the queue for the miner.
func (t *TransactionQueueMemory) Get(limit int) []*Transaction {
	var txs []*Transaction
	t.lock.RLock()
	defer t.lock.RUnlock()
	if limit == 0 || t.txQueue.Len() < limit {
		txs = make([]*Transaction, t.txQueue.Len())
	} else {
		txs = make([]*Transaction, limit)
	}
	i := 0
	for e := t.txQueue.Front(); e != nil; e = e.Next() {
		txs[i] = e.Value.(*Transaction)
		i++
		if i == limit {
			break
		}
	}
	return txs
}

// Exists returns true if the given transaction is in the queue.
func (t *TransactionQueueMemory) Exists(id TransactionID) bool {
	t.lock.RLock()
	defer t.lock.RUnlock()
	_, ok := t.txMap[id]
	return ok
}

// ExistsSigned returns true if the given transaction is in the queue and contains the given signature.
func (t *TransactionQueueMemory) ExistsSigned(id TransactionID, signature Signature) bool {
	t.lock.RLock()
	defer t.lock.RUnlock()
	if e, ok := t.txMap[id]; ok {
		tx := e.Value.(*Transaction)
		return bytes.Equal(tx.Signature, signature)
	}
	return false
}

// Len returns the queue length.
func (t *TransactionQueueMemory) Len() int {
	t.lock.RLock()
	defer t.lock.RUnlock()
	return t.txQueue.Len()
}
