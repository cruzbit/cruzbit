// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

// BlockStorage is an interface for storing blocks and their transactions.
type BlockStorage interface {
	// Store is called to store all of the block's information.
	Store(id BlockID, block *Block, now int64) error

	// Get returns the referenced block.
	GetBlock(id BlockID) (*Block, error)

	// GetBlockBytes returns the referenced block as a byte slice.
	GetBlockBytes(id BlockID) ([]byte, error)

	// GetBlockHeader returns the referenced block's header and the timestamp of when it was stored.
	GetBlockHeader(id BlockID) (*BlockHeader, int64, error)

	// GetTransaction returns a transaction within a block and the block's header.
	GetTransaction(id BlockID, index int) (*Transaction, *BlockHeader, error)
}
