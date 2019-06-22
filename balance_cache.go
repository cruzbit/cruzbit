// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"golang.org/x/crypto/ed25519"
)

// BalanceCache maintains a partial unconfirmed view of the ledger.
// It's used by Ledger when (dis-)connecting blocks and by TransactionQueueMemory
// when deciding whether or not to add a transaction to the queue.
type BalanceCache struct {
	ledger     Ledger
	minBalance int64
	cache      map[[ed25519.PublicKeySize]byte]int64
}

// NewBalanceCache returns a new instance of a BalanceCache.
func NewBalanceCache(ledger Ledger, minBalance int64) *BalanceCache {
	b := &BalanceCache{ledger: ledger, minBalance: minBalance}
	b.Reset()
	return b
}

// Reset resets the balance cache.
func (b *BalanceCache) Reset() {
	b.cache = make(map[[ed25519.PublicKeySize]byte]int64)
}

// Apply applies the effect of the transaction to the invovled parties' cached balances.
// It returns false if sender balance would go negative as a result of applying this transaction.
// It also returns false if a remaining non-zero sender balance would be less than minBalance.
func (b *BalanceCache) Apply(tx *Transaction) (bool, error) {
	if !tx.IsCoinbase() {
		// check and debit sender balance
		var fpk [ed25519.PublicKeySize]byte
		copy(fpk[:], tx.From)
		senderBalance, ok := b.cache[fpk]
		if !ok {
			var err error
			senderBalance, err = b.ledger.GetPublicKeyBalance(tx.From)
			if err != nil {
				return false, err
			}
		}
		totalSpent := tx.Amount + tx.Fee
		if totalSpent > senderBalance {
			return false, nil
		}
		senderBalance -= totalSpent
		if senderBalance > 0 && senderBalance < b.minBalance {
			return false, nil
		}
		b.cache[fpk] = senderBalance
	}

	// credit recipient balance
	var tpk [ed25519.PublicKeySize]byte
	copy(tpk[:], tx.To)
	recipientBalance, ok := b.cache[tpk]
	if !ok {
		var err error
		recipientBalance, err = b.ledger.GetPublicKeyBalance(tx.To)
		if err != nil {
			return false, err
		}
	}
	recipientBalance += tx.Amount
	b.cache[tpk] = recipientBalance
	return true, nil
}

// Undo undoes the effects of a transaction on the invovled parties' cached balances.
func (b *BalanceCache) Undo(tx *Transaction) error {
	if !tx.IsCoinbase() {
		// credit balance for sender
		var fpk [ed25519.PublicKeySize]byte
		copy(fpk[:], tx.From)
		senderBalance, ok := b.cache[fpk]
		if !ok {
			var err error
			senderBalance, err = b.ledger.GetPublicKeyBalance(tx.From)
			if err != nil {
				return err
			}
		}
		totalSpent := tx.Amount + tx.Fee
		senderBalance += totalSpent
		b.cache[fpk] = senderBalance
	}

	// debit recipient balance
	var tpk [ed25519.PublicKeySize]byte
	copy(tpk[:], tx.To)
	recipientBalance, ok := b.cache[tpk]
	if !ok {
		var err error
		recipientBalance, err = b.ledger.GetPublicKeyBalance(tx.To)
		if err != nil {
			return err
		}
	}
	if recipientBalance < tx.Amount {
		panic("Recipient balance went negative")
	}
	b.cache[tpk] = recipientBalance - tx.Amount
	return nil
}

// Balances returns the underlying cache of balances.
func (b *BalanceCache) Balances() map[[ed25519.PublicKeySize]byte]int64 {
	return b.cache
}
