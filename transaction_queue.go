// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

// TransactionQueue is an interface to a queue of transactions to be confirmed.
type TransactionQueue interface {
	// Add adds the transaction to the queue. Returns true if the transaction was added to the queue on this call.
	Add(id TransactionID, tx *Transaction) (bool, error)

	// AddBatch adds a batch of transactions to the queue (a block has been disconnected.)
	// "height" is the block chain height after this disconnection.
	AddBatch(ids []TransactionID, txs []*Transaction, height int64) error

	// RemoveBatch removes a batch of transactions from the queue (a block has been connected.)
	// "height" is the block chain height after this connection.
	// "more" indicates if more connections are coming.
	RemoveBatch(ids []TransactionID, height int64, more bool) error

	// Get returns transactions in the queue for the miner.
	Get(limit int) []*Transaction

	// Exists returns true if the given transaction is in the queue.
	Exists(id TransactionID) bool

	// ExistsSigned returns true if the given transaction is in the queue and contains the given signature.
	ExistsSigned(id TransactionID, signature Signature) bool

	// Len returns the queue length.
	Len() int
}
