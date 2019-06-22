// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"golang.org/x/crypto/ed25519"
)

// BranchType indicates the type of branch a particular block resides on.
// Only blocks currently on the main branch are considered confirmed and only
// transactions in those blocks affect public key balances.
// Values are: MAIN, SIDE, ORPHAN or UNKNOWN.
type BranchType int

const (
	MAIN = iota
	SIDE
	ORPHAN
	UNKNOWN
)

// Ledger is an interface to a ledger built from the most-work chain of blocks.
// It manages and computes public key balances as well as transaction and public key transaction indices.
// It also maintains an index of the block chain by height as well as branch information.
type Ledger interface {
	// GetChainTip returns the ID and the height of the block at the current tip of the main chain.
	GetChainTip() (*BlockID, int64, error)

	// GetBlockIDForHeight returns the ID of the block at the given block chain height.
	GetBlockIDForHeight(height int64) (*BlockID, error)

	// SetBranchType sets the branch type for the given block.
	SetBranchType(id BlockID, branchType BranchType) error

	// GetBranchType returns the branch type for the given block.
	GetBranchType(id BlockID) (BranchType, error)

	// ConnectBlock connects a block to the tip of the block chain and applies the transactions
	// to the ledger.
	ConnectBlock(id BlockID, block *Block) ([]TransactionID, error)

	// DisconnectBlock disconnects a block from the tip of the block chain and undoes the effects
	// of the transactions on the ledger.
	DisconnectBlock(id BlockID, block *Block) ([]TransactionID, error)

	// GetPublicKeyBalance returns the current balance of a given public key.
	GetPublicKeyBalance(pubKey ed25519.PublicKey) (int64, error)

	// GetPublicKeyBalances returns the current balance of the given public keys
	// along with block ID and height of the corresponding main chain tip.
	GetPublicKeyBalances(pubKeys []ed25519.PublicKey) (
		map[[ed25519.PublicKeySize]byte]int64, *BlockID, int64, error)

	// GetTransactionIndex returns the index of a processed transaction.
	GetTransactionIndex(id TransactionID) (*BlockID, int, error)

	// GetPublicKeyTransactionIndicesRange returns transaction indices involving a given public key
	// over a range of heights. If startHeight > endHeight this iterates in reverse.
	GetPublicKeyTransactionIndicesRange(
		pubKey ed25519.PublicKey, startHeight, endHeight int64, startIndex, limit int) (
		[]BlockID, []int, int64, int, error)

	// Balance returns the total current ledger balance by summing the balance of all public keys.
	// It's only used offline for verification purposes.
	Balance() (int64, error)

	// GetPublicKeyBalanceAt returns the public key balance at the given height.
	// It's only used offline for historical and verification purposes.
	// This is only accurate when the full block chain is indexed (pruning disabled.)
	GetPublicKeyBalanceAt(pubKey ed25519.PublicKey, height int64) (int64, error)
}
