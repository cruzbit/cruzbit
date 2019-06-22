// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/sha3"
)

// Transaction represents a ledger transaction. It transfers value from one public key to another.
type Transaction struct {
	Time      int64             `json:"time"`
	Nonce     int32             `json:"nonce"` // collision prevention. pseudorandom. not used for crypto
	From      ed25519.PublicKey `json:"from,omitempty"`
	To        ed25519.PublicKey `json:"to"`
	Amount    int64             `json:"amount"`
	Fee       int64             `json:"fee,omitempty"`
	Memo      string            `json:"memo,omitempty"`    // max 100 characters
	Matures   int64             `json:"matures,omitempty"` // block height. if set transaction can't be mined before
	Expires   int64             `json:"expires,omitempty"` // block height. if set transaction can't be mined after
	Series    int64             `json:"series"`            // +1 roughly once a week to allow for pruning history
	Signature Signature         `json:"signature,omitempty"`
}

// TransactionID is a transaction's unique identifier.
type TransactionID [32]byte // SHA3-256 hash

// Signature is a transaction's signature.
type Signature []byte

// NewTransaction returns a new unsigned transaction.
func NewTransaction(from, to ed25519.PublicKey, amount, fee, matures, expires, height int64, memo string) *Transaction {
	return &Transaction{
		Time:    time.Now().Unix(),
		Nonce:   rand.Int31(),
		From:    from,
		To:      to,
		Amount:  amount,
		Fee:     fee,
		Memo:    memo,
		Matures: matures,
		Expires: expires,
		Series:  computeTransactionSeries(from == nil, height),
	}
}

// ID computes an ID for a given transaction.
func (tx Transaction) ID() (TransactionID, error) {
	// never include the signature in the ID
	// this way we never have to think about signature malleability
	tx.Signature = nil
	txJson, err := json.Marshal(tx)
	if err != nil {
		return TransactionID{}, err
	}
	return sha3.Sum256([]byte(txJson)), nil
}

// Sign is called to sign a transaction.
func (tx *Transaction) Sign(privKey ed25519.PrivateKey) error {
	id, err := tx.ID()
	if err != nil {
		return err
	}
	tx.Signature = ed25519.Sign(privKey, id[:])
	return nil
}

// Verify is called to verify only that the transaction is properly signed.
func (tx Transaction) Verify() (bool, error) {
	id, err := tx.ID()
	if err != nil {
		return false, err
	}
	return ed25519.Verify(tx.From, id[:], tx.Signature), nil
}

// IsCoinbase returns true if the transaction is a coinbase. A coinbase is the first transaction in every block
// used to reward the miner for mining the block.
func (tx Transaction) IsCoinbase() bool {
	return tx.From == nil
}

// Contains returns true if the transaction is relevant to the given public key.
func (tx Transaction) Contains(pubKey ed25519.PublicKey) bool {
	if !tx.IsCoinbase() {
		if bytes.Equal(pubKey, tx.From) {
			return true
		}
	}
	return bytes.Equal(pubKey, tx.To)
}

// IsMature returns true if the transaction can be mined at the given height.
func (tx Transaction) IsMature(height int64) bool {
	if tx.Matures == 0 {
		return true
	}
	return tx.Matures >= height
}

// IsExpired returns true if the transaction cannot be mined at the given height.
func (tx Transaction) IsExpired(height int64) bool {
	if tx.Expires == 0 {
		return false
	}
	return tx.Expires < height
}

// String implements the Stringer interface.
func (id TransactionID) String() string {
	return hex.EncodeToString(id[:])
}

// MarshalJSON marshals TransactionID as a hex string.
func (id TransactionID) MarshalJSON() ([]byte, error) {
	s := "\"" + id.String() + "\""
	return []byte(s), nil
}

// UnmarshalJSON unmarshals a hex string to TransactionID.
func (id *TransactionID) UnmarshalJSON(b []byte) error {
	if len(b) != 64+2 {
		return fmt.Errorf("Invalid transaction ID")
	}
	idBytes, err := hex.DecodeString(string(b[1 : len(b)-1]))
	if err != nil {
		return err
	}
	copy(id[:], idBytes)
	return nil
}

// Compute the series to use for a new transaction.
func computeTransactionSeries(isCoinbase bool, height int64) int64 {
	if isCoinbase {
		// coinbases start using the new series right on time
		return height/BLOCKS_UNTIL_NEW_SERIES + 1
	}

	// otherwise don't start using a new series until 100 blocks in to mitigate
	// potential reorg issues right around the switchover
	return (height-100)/BLOCKS_UNTIL_NEW_SERIES + 1
}
