// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
	"golang.org/x/crypto/ed25519"
)

// LedgerDisk is an on-disk implemenation of the Ledger interface using LevelDB.
type LedgerDisk struct {
	db         *leveldb.DB
	blockStore BlockStorage
	prune      bool // prune historic transaction and public key transaction indices
}

// NewLedgerDisk returns a new instance of LedgerDisk.
func NewLedgerDisk(dbPath string, readOnly, prune bool, blockStore BlockStorage) (*LedgerDisk, error) {
	opts := opt.Options{ReadOnly: readOnly}
	db, err := leveldb.OpenFile(dbPath, &opts)
	if err != nil {
		return nil, err
	}
	return &LedgerDisk{db: db, blockStore: blockStore, prune: prune}, nil
}

// GetChainTip returns the ID and the height of the block at the current tip of the main chain.
func (l LedgerDisk) GetChainTip() (*BlockID, int64, error) {
	return getChainTip(l.db)
}

// Sometimes we call this with *leveldb.DB or *leveldb.Snapshot
func getChainTip(db leveldb.Reader) (*BlockID, int64, error) {
	// compute db key
	key, err := computeChainTipKey()
	if err != nil {
		return nil, 0, err
	}

	// fetch the id
	ctBytes, err := db.Get(key, nil)
	if err == leveldb.ErrNotFound {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}

	// decode the tip
	id, height, err := decodeChainTip(ctBytes)
	if err != nil {
		return nil, 0, err
	}

	return id, height, nil
}

// GetBlockIDForHeight returns the ID of the block at the given block chain height.
func (l LedgerDisk) GetBlockIDForHeight(height int64) (*BlockID, error) {
	return getBlockIDForHeight(height, l.db)
}

// Sometimes we call this with *leveldb.DB or *leveldb.Snapshot
func getBlockIDForHeight(height int64, db leveldb.Reader) (*BlockID, error) {
	// compute db key
	key, err := computeBlockHeightIndexKey(height)
	if err != nil {
		return nil, err
	}

	// fetch the id
	idBytes, err := db.Get(key, nil)
	if err == leveldb.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// return it
	id := new(BlockID)
	copy(id[:], idBytes)
	return id, nil
}

// SetBranchType sets the branch type for the given block.
func (l LedgerDisk) SetBranchType(id BlockID, branchType BranchType) error {
	// compute db key
	key, err := computeBranchTypeKey(id)
	if err != nil {
		return err
	}

	// write type
	wo := opt.WriteOptions{Sync: true}
	return l.db.Put(key, []byte{byte(branchType)}, &wo)
}

// GetBranchType returns the branch type for the given block.
func (l LedgerDisk) GetBranchType(id BlockID) (BranchType, error) {
	// compute db key
	key, err := computeBranchTypeKey(id)
	if err != nil {
		return UNKNOWN, err
	}

	// fetch type
	branchType, err := l.db.Get(key, nil)
	if err == leveldb.ErrNotFound {
		return UNKNOWN, nil
	}
	if err != nil {
		return UNKNOWN, err
	}
	return BranchType(branchType[0]), nil
}

// ConnectBlock connects a block to the tip of the block chain and applies the transactions to the ledger.
func (l LedgerDisk) ConnectBlock(id BlockID, block *Block) ([]TransactionID, error) {
	// sanity check
	tipID, _, err := l.GetChainTip()
	if err != nil {
		return nil, err
	}
	if tipID != nil && *tipID != block.Header.Previous {
		return nil, fmt.Errorf("Being asked to connect %s but previous %s does not match tip %s",
			id, block.Header.Previous, *tipID)
	}

	// apply all resulting writes atomically
	batch := new(leveldb.Batch)

	balanceCache := NewBalanceCache(l, 0)
	txIDs := make([]TransactionID, len(block.Transactions))

	for i, tx := range block.Transactions {
		txID, err := tx.ID()
		if err != nil {
			return nil, err
		}
		txIDs[i] = txID

		// verify the transaction hasn't been processed already.
		// note that we can safely prune indices for transactions older than the previous series
		key, err := computeTransactionIndexKey(txID)
		if err != nil {
			return nil, err
		}
		ok, err := l.db.Has(key, nil)
		if err != nil {
			return nil, err
		}
		if ok {
			return nil, fmt.Errorf("Transaction %s already processed", txID)
		}

		// set the transaction index now
		indexBytes, err := encodeTransactionIndex(block.Header.Height, i)
		if err != nil {
			return nil, err
		}
		batch.Put(key, indexBytes)

		txToApply := tx

		if tx.IsCoinbase() {
			// don't apply a coinbase to a balance until it's 100 blocks deep.
			// during honest reorgs normal transactions usually get into the new most-work branch
			// but coinbases vanish. this mitigates the impact on UX when reorgs occur and transactions
			// depend on coinbases.
			txToApply = nil

			if block.Header.Height-COINBASE_MATURITY >= 0 {
				// mature the coinbase from 100 blocks ago now
				oldID, err := l.GetBlockIDForHeight(block.Header.Height - COINBASE_MATURITY)
				if err != nil {
					return nil, err
				}
				if oldID == nil {
					return nil, fmt.Errorf("Missing block at height %d\n",
						block.Header.Height-COINBASE_MATURITY)
				}

				// we could store the last 100 coinbases on our own in memory if we end up needing to
				oldTx, _, err := l.blockStore.GetTransaction(*oldID, 0)
				if err != nil {
					return nil, err
				}
				if oldTx == nil {
					return nil, fmt.Errorf("Missing coinbase from block %s\n", *oldID)
				}

				// apply it to the recipient's balance
				txToApply = oldTx
			}
		}

		if txToApply != nil {
			// check sender balance and update sender and receiver balances
			ok, err := balanceCache.Apply(txToApply)
			if err != nil {
				return nil, err
			}
			if !ok {
				txID, _ := txToApply.ID()
				return nil, fmt.Errorf("Sender has insuffcient balance in transaction %s", txID)
			}
		}

		// associate this transaction with both parties
		if !tx.IsCoinbase() {
			key, err = computePubKeyTransactionIndexKey(tx.From, &block.Header.Height, &i)
			if err != nil {
				return nil, err
			}
			batch.Put(key, []byte{0x1})
		}
		key, err = computePubKeyTransactionIndexKey(tx.To, &block.Header.Height, &i)
		if err != nil {
			return nil, err
		}
		batch.Put(key, []byte{0x1})
	}

	// update recorded balances
	balances := balanceCache.Balances()
	for pubKeyBytes, balance := range balances {
		key, err := computePubKeyBalanceKey(ed25519.PublicKey(pubKeyBytes[:]))
		if err != nil {
			return nil, err
		}
		if balance == 0 {
			batch.Delete(key)
		} else {
			balanceBytes, err := encodeNumber(balance)
			if err != nil {
				return nil, err
			}
			batch.Put(key, balanceBytes)
		}
	}

	// index the block by height
	key, err := computeBlockHeightIndexKey(block.Header.Height)
	if err != nil {
		return nil, err
	}
	batch.Put(key, id[:])

	// set this block on the main chain
	key, err = computeBranchTypeKey(id)
	if err != nil {
		return nil, err
	}
	batch.Put(key, []byte{byte(MAIN)})

	// set this block as the new tip
	key, err = computeChainTipKey()
	if err != nil {
		return nil, err
	}
	ctBytes, err := encodeChainTip(id, block.Header.Height)
	if err != nil {
		return nil, err
	}
	batch.Put(key, ctBytes)

	// prune historic transaction and public key transaction indices now
	if l.prune && block.Header.Height >= 2*BLOCKS_UNTIL_NEW_SERIES {
		if err := l.pruneIndices(block.Header.Height-2*BLOCKS_UNTIL_NEW_SERIES, batch); err != nil {
			return nil, err
		}
	}

	// perform the writes
	wo := opt.WriteOptions{Sync: true}
	if err := l.db.Write(batch, &wo); err != nil {
		return nil, err
	}

	return txIDs, nil
}

// DisconnectBlock disconnects a block from the tip of the block chain and undoes the effects of the transactions on the ledger.
func (l LedgerDisk) DisconnectBlock(id BlockID, block *Block) ([]TransactionID, error) {
	// sanity check
	tipID, _, err := l.GetChainTip()
	if err != nil {
		return nil, err
	}
	if tipID == nil {
		return nil, fmt.Errorf("Being asked to disconnect %s but no tip is currently set",
			id)
	}
	if *tipID != id {
		return nil, fmt.Errorf("Being asked to disconnect %s but it does not match tip %s",
			id, *tipID)
	}

	// apply all resulting writes atomically
	batch := new(leveldb.Batch)

	balanceCache := NewBalanceCache(l, 0)
	txIDs := make([]TransactionID, len(block.Transactions))

	// disconnect transactions in reverse order
	for i := len(block.Transactions) - 1; i >= 0; i-- {
		tx := block.Transactions[i]
		txID, err := tx.ID()
		if err != nil {
			return nil, err
		}
		// save the id
		txIDs[i] = txID

		// mark the transaction unprocessed now (delete its index)
		key, err := computeTransactionIndexKey(txID)
		if err != nil {
			return nil, err
		}
		batch.Delete(key)

		txToUndo := tx
		if tx.IsCoinbase() {
			// coinbase doesn't affect recipient balance for 100 more blocks
			txToUndo = nil

			if block.Header.Height-COINBASE_MATURITY >= 0 {
				// undo the effect of the coinbase from 100 blocks ago now
				oldID, err := l.GetBlockIDForHeight(block.Header.Height - COINBASE_MATURITY)
				if err != nil {
					return nil, err
				}
				if oldID == nil {
					return nil, fmt.Errorf("Missing block at height %d\n",
						block.Header.Height-COINBASE_MATURITY)
				}
				oldTx, _, err := l.blockStore.GetTransaction(*oldID, 0)
				if err != nil {
					return nil, err
				}
				if oldTx == nil {
					return nil, fmt.Errorf("Missing coinbase from block %s\n", *oldID)
				}

				// undo its effect on the recipient's balance
				txToUndo = oldTx
			}
		}

		if txToUndo != nil {
			// credit sender and debit recipient
			err = balanceCache.Undo(txToUndo)
			if err != nil {
				return nil, err
			}
		}

		// unassociate this transaction with both parties
		if !tx.IsCoinbase() {
			key, err = computePubKeyTransactionIndexKey(tx.From, &block.Header.Height, &i)
			if err != nil {
				return nil, err
			}
			batch.Delete(key)
		}
		key, err = computePubKeyTransactionIndexKey(tx.To, &block.Header.Height, &i)
		if err != nil {
			return nil, err
		}
		batch.Delete(key)
	}

	// update recorded balances
	balances := balanceCache.Balances()
	for pubKeyBytes, balance := range balances {
		key, err := computePubKeyBalanceKey(ed25519.PublicKey(pubKeyBytes[:]))
		if err != nil {
			return nil, err
		}
		if balance == 0 {
			batch.Delete(key)
		} else {
			balanceBytes, err := encodeNumber(balance)
			if err != nil {
				return nil, err
			}
			batch.Put(key, balanceBytes)
		}
	}

	// remove this block's index by height
	key, err := computeBlockHeightIndexKey(block.Header.Height)
	if err != nil {
		return nil, err
	}
	batch.Delete(key)

	// set this block on a side chain
	key, err = computeBranchTypeKey(id)
	if err != nil {
		return nil, err
	}
	batch.Put(key, []byte{byte(SIDE)})

	// set the previous block as the chain tip
	key, err = computeChainTipKey()
	if err != nil {
		return nil, err
	}
	ctBytes, err := encodeChainTip(block.Header.Previous, block.Header.Height-1)
	if err != nil {
		return nil, err
	}
	batch.Put(key, ctBytes)

	// restore historic indices now
	if l.prune && block.Header.Height >= 2*BLOCKS_UNTIL_NEW_SERIES {
		if err := l.restoreIndices(block.Header.Height-2*BLOCKS_UNTIL_NEW_SERIES, batch); err != nil {
			return nil, err
		}
	}

	// perform the writes
	wo := opt.WriteOptions{Sync: true}
	if err := l.db.Write(batch, &wo); err != nil {
		return nil, err
	}

	return txIDs, nil
}

// Prune transaction and public key transaction indices created by the block at the given height
func (l LedgerDisk) pruneIndices(height int64, batch *leveldb.Batch) error {
	// get the ID
	id, err := l.GetBlockIDForHeight(height)
	if err != nil {
		return err
	}
	if id == nil {
		return fmt.Errorf("Missing block ID for height %d\n", height)
	}

	// fetch the block
	block, err := l.blockStore.GetBlock(*id)
	if err != nil {
		return err
	}
	if block == nil {
		return fmt.Errorf("Missing block %s\n", *id)
	}

	for i, tx := range block.Transactions {
		txID, err := tx.ID()
		if err != nil {
			return err
		}

		// prune transaction index
		key, err := computeTransactionIndexKey(txID)
		if err != nil {
			return err
		}
		batch.Delete(key)

		// prune public key transaction indices
		if !tx.IsCoinbase() {
			key, err = computePubKeyTransactionIndexKey(tx.From, &block.Header.Height, &i)
			if err != nil {
				return err
			}
			batch.Delete(key)
		}
		key, err = computePubKeyTransactionIndexKey(tx.To, &block.Header.Height, &i)
		if err != nil {
			return err
		}
		batch.Delete(key)
	}

	return nil
}

// Restore transaction and public key transaction indices created by the block at the given height
func (l LedgerDisk) restoreIndices(height int64, batch *leveldb.Batch) error {
	// get the ID
	id, err := l.GetBlockIDForHeight(height)
	if err != nil {
		return err
	}
	if id == nil {
		return fmt.Errorf("Missing block ID for height %d\n", height)
	}

	// fetch the block
	block, err := l.blockStore.GetBlock(*id)
	if err != nil {
		return err
	}
	if block == nil {
		return fmt.Errorf("Missing block %s\n", *id)
	}

	for i, tx := range block.Transactions {
		txID, err := tx.ID()
		if err != nil {
			return err
		}

		// restore transaction index
		key, err := computeTransactionIndexKey(txID)
		if err != nil {
			return err
		}
		indexBytes, err := encodeTransactionIndex(block.Header.Height, i)
		if err != nil {
			return err
		}
		batch.Put(key, indexBytes)

		// restore public key transaction indices
		if !tx.IsCoinbase() {
			key, err = computePubKeyTransactionIndexKey(tx.From, &block.Header.Height, &i)
			if err != nil {
				return err
			}
			batch.Put(key, []byte{0x1})
		}
		key, err = computePubKeyTransactionIndexKey(tx.To, &block.Header.Height, &i)
		if err != nil {
			return err
		}
		batch.Put(key, []byte{0x1})
	}

	return nil
}

// GetPublicKeyBalance returns the current balance of a given public key.
func (l LedgerDisk) GetPublicKeyBalance(pubKey ed25519.PublicKey) (int64, error) {
	// compute db key
	key, err := computePubKeyBalanceKey(pubKey)
	if err != nil {
		return 0, err
	}

	// fetch balance
	balanceBytes, err := l.db.Get(key, nil)
	if err == leveldb.ErrNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	// decode and return it
	var balance int64
	buf := bytes.NewReader(balanceBytes)
	binary.Read(buf, binary.BigEndian, &balance)
	return balance, nil
}

// GetPublicKeyBalances returns the current balance of the given public keys
// along with block ID and height of the corresponding main chain tip.
func (l LedgerDisk) GetPublicKeyBalances(pubKeys []ed25519.PublicKey) (
	map[[ed25519.PublicKeySize]byte]int64, *BlockID, int64, error) {

	// get a consistent view across all queries
	snapshot, err := l.db.GetSnapshot()
	if err != nil {
		return nil, nil, 0, err
	}
	defer snapshot.Release()

	// get the chain tip
	tipID, tipHeight, err := getChainTip(snapshot)
	if err != nil {
		return nil, nil, 0, err
	}

	balances := make(map[[ed25519.PublicKeySize]byte]int64)

	for _, pubKey := range pubKeys {
		// compute balance db key
		key, err := computePubKeyBalanceKey(pubKey)
		if err != nil {
			return nil, nil, 0, err
		}

		var pk [ed25519.PublicKeySize]byte
		copy(pk[:], pubKey)

		// fetch balance
		balanceBytes, err := snapshot.Get(key, nil)
		if err == leveldb.ErrNotFound {
			balances[pk] = 0
			continue
		}
		if err != nil {
			return nil, nil, 0, err
		}

		// decode it
		var balance int64
		buf := bytes.NewReader(balanceBytes)
		binary.Read(buf, binary.BigEndian, &balance)

		// save it
		balances[pk] = balance
	}

	return balances, tipID, tipHeight, nil
}

// GetTransactionIndex returns the index of a processed transaction.
func (l LedgerDisk) GetTransactionIndex(id TransactionID) (*BlockID, int, error) {
	// compute the db key
	key, err := computeTransactionIndexKey(id)
	if err != nil {
		return nil, 0, err
	}

	// we want a consistent view during our two queries as height can change
	snapshot, err := l.db.GetSnapshot()
	if err != nil {
		return nil, 0, err
	}
	defer snapshot.Release()

	// fetch and decode the index
	indexBytes, err := snapshot.Get(key, nil)
	if err == leveldb.ErrNotFound {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	height, index, err := decodeTransactionIndex(indexBytes)
	if err != nil {
		return nil, 0, err
	}

	// map height to block id
	blockID, err := getBlockIDForHeight(height, snapshot)
	if err != nil {
		return nil, 0, err
	}

	// return it
	return blockID, index, nil
}

// GetPublicKeyTransactionIndicesRange returns transaction indices involving a given public key
// over a range of heights. If startHeight > endHeight this iterates in reverse.
func (l LedgerDisk) GetPublicKeyTransactionIndicesRange(
	pubKey ed25519.PublicKey, startHeight, endHeight int64, startIndex, limit int) (
	[]BlockID, []int, int64, int, error) {

	if endHeight >= startHeight {
		// forward
		return l.getPublicKeyTransactionIndicesRangeForward(
			pubKey, startHeight, endHeight, startIndex, limit)
	}

	// reverse
	return l.getPublicKeyTransactionIndicesRangeReverse(
		pubKey, startHeight, endHeight, startIndex, limit)
}

// Iterate through transaction history going forward
func (l LedgerDisk) getPublicKeyTransactionIndicesRangeForward(
	pubKey ed25519.PublicKey, startHeight, endHeight int64, startIndex, limit int) (
	ids []BlockID, indices []int, lastHeight int64, lastIndex int, err error) {
	startKey, err := computePubKeyTransactionIndexKey(pubKey, &startHeight, &startIndex)
	if err != nil {
		return
	}

	endHeight += 1 // make it inclusive
	endKey, err := computePubKeyTransactionIndexKey(pubKey, &endHeight, nil)
	if err != nil {
		return
	}

	heightMap := make(map[int64]*BlockID)

	// we want a consistent view of this. heights can change out from under us otherwise
	snapshot, err := l.db.GetSnapshot()
	if err != nil {
		return
	}
	defer snapshot.Release()

	iter := snapshot.NewIterator(&util.Range{Start: startKey, Limit: endKey}, nil)
	for iter.Next() {
		_, lastHeight, lastIndex, err = decodePubKeyTransactionIndexKey(iter.Key())
		if err != nil {
			iter.Release()
			return nil, nil, 0, 0, err
		}

		// lookup the block id
		id, ok := heightMap[lastHeight]
		if !ok {
			var err error
			id, err = getBlockIDForHeight(lastHeight, snapshot)
			if err != nil {
				iter.Release()
				return nil, nil, 0, 0, err
			}
			if id == nil {
				iter.Release()
				return nil, nil, 0, 0, fmt.Errorf(
					"No block found at height %d", lastHeight)
			}
			heightMap[lastHeight] = id
		}

		ids = append(ids, *id)
		indices = append(indices, lastIndex)
		if limit != 0 && len(indices) == limit {
			break
		}
	}
	iter.Release()
	if err := iter.Error(); err != nil {
		return nil, nil, 0, 0, err
	}
	return
}

// Iterate through transaction history in reverse
func (l LedgerDisk) getPublicKeyTransactionIndicesRangeReverse(
	pubKey ed25519.PublicKey, startHeight, endHeight int64, startIndex, limit int) (
	ids []BlockID, indices []int, lastHeight int64, lastIndex int, err error) {
	endKey, err := computePubKeyTransactionIndexKey(pubKey, &endHeight, nil)
	if err != nil {
		return
	}

	// make it inclusive
	startIndex += 1
	startKey, err := computePubKeyTransactionIndexKey(pubKey, &startHeight, &startIndex)
	if err != nil {
		return
	}

	heightMap := make(map[int64]*BlockID)

	// we want a consistent view of this. heights can change out from under us otherwise
	snapshot, err := l.db.GetSnapshot()
	if err != nil {
		return
	}
	defer snapshot.Release()

	iter := snapshot.NewIterator(&util.Range{Start: endKey, Limit: startKey}, nil)
	for ok := iter.Last(); ok; ok = iter.Prev() {
		_, lastHeight, lastIndex, err = decodePubKeyTransactionIndexKey(iter.Key())
		if err != nil {
			iter.Release()
			return nil, nil, 0, 0, err
		}

		// lookup the block id
		id, ok := heightMap[lastHeight]
		if !ok {
			var err error
			id, err = getBlockIDForHeight(lastHeight, snapshot)
			if err != nil {
				iter.Release()
				return nil, nil, 0, 0, err
			}
			if id == nil {
				iter.Release()
				return nil, nil, 0, 0, fmt.Errorf(
					"No block found at height %d", lastHeight)
			}
			heightMap[lastHeight] = id
		}

		ids = append(ids, *id)
		indices = append(indices, lastIndex)
		if limit != 0 && len(indices) == limit {
			break
		}
	}
	iter.Release()
	if err := iter.Error(); err != nil {
		return nil, nil, 0, 0, err
	}
	return
}

// Balance returns the total current ledger balance by summing the balance of all public keys.
// It's only used offline for verification purposes.
func (l LedgerDisk) Balance() (int64, error) {
	var total int64

	// compute the sum of all public key balances
	key, err := computePubKeyBalanceKey(nil)
	if err != nil {
		return 0, err
	}
	iter := l.db.NewIterator(util.BytesPrefix(key), nil)
	for iter.Next() {
		var balance int64
		buf := bytes.NewReader(iter.Value())
		binary.Read(buf, binary.BigEndian, &balance)
		total += balance
	}
	iter.Release()
	if err := iter.Error(); err != nil {
		return 0, err
	}

	return total, nil
}

// GetPublicKeyBalanceAt returns the public key balance at the given height.
// It's only used offline for historical and verification purposes.
// This is only accurate when the full block chain is indexed (pruning disabled.)
func (l LedgerDisk) GetPublicKeyBalanceAt(pubKey ed25519.PublicKey, height int64) (int64, error) {
	_, currentHeight, err := l.GetChainTip()
	if err != nil {
		return 0, err
	}

	startKey, err := computePubKeyTransactionIndexKey(pubKey, nil, nil)
	if err != nil {
		return 0, err
	}

	height += 1 // make it inclusive
	endKey, err := computePubKeyTransactionIndexKey(pubKey, &height, nil)
	if err != nil {
		return 0, err
	}

	var balance int64
	iter := l.db.NewIterator(&util.Range{Start: startKey, Limit: endKey}, nil)
	for iter.Next() {
		_, height, index, err := decodePubKeyTransactionIndexKey(iter.Key())
		if err != nil {
			iter.Release()
			return 0, err
		}

		if index == 0 && height > currentHeight-COINBASE_MATURITY {
			// coinbase isn't mature
			continue
		}

		id, err := l.GetBlockIDForHeight(height)
		if err != nil {
			iter.Release()
			return 0, err
		}
		if id == nil {
			iter.Release()
			return 0, fmt.Errorf("No block found at height %d", height)
		}

		tx, _, err := l.blockStore.GetTransaction(*id, index)
		if err != nil {
			iter.Release()
			return 0, err
		}
		if tx == nil {
			iter.Release()
			return 0, fmt.Errorf("No transaction found in block %s at index %d",
				*id, index)
		}

		if bytes.Equal(pubKey, tx.To) {
			balance += tx.Amount
		} else if bytes.Equal(pubKey, tx.From) {
			balance -= tx.Amount
			balance -= tx.Fee
			if balance < 0 {
				iter.Release()
				txID, _ := tx.ID()
				return 0, fmt.Errorf("Balance went negative at transaction %s", txID)
			}
		} else {
			iter.Release()
			txID, _ := tx.ID()
			return 0, fmt.Errorf("Transaction %s doesn't involve the public key", txID)
		}
	}
	iter.Release()
	if err := iter.Error(); err != nil {
		return 0, err
	}
	return balance, nil
}

// Close is called to close any underlying storage.
func (l LedgerDisk) Close() error {
	return l.db.Close()
}

// leveldb schema

// T                    -> {bid}{height} (main chain tip)
// B{bid}               -> main|side|orphan (1 byte)
// h{height}            -> {bid}
// t{txid}              -> {height}{index} (prunable up to the previous series)
// k{pk}{height}{index} -> 1 (not strictly necessary. probably should make it optional by flag)
// b{pk}                -> {balance} (we always need all of this table)

const chainTipPrefix = 'T'

const branchTypePrefix = 'B'

const blockHeightIndexPrefix = 'h'

const transactionIndexPrefix = 't'

const pubKeyTransactionIndexPrefix = 'k'

const pubKeyBalancePrefix = 'b'

func computeBranchTypeKey(id BlockID) ([]byte, error) {
	key := new(bytes.Buffer)
	if err := key.WriteByte(branchTypePrefix); err != nil {
		return nil, err
	}
	if err := binary.Write(key, binary.BigEndian, id[:]); err != nil {
		return nil, err
	}
	return key.Bytes(), nil
}

func computeBlockHeightIndexKey(height int64) ([]byte, error) {
	key := new(bytes.Buffer)
	if err := key.WriteByte(blockHeightIndexPrefix); err != nil {
		return nil, err
	}
	if err := binary.Write(key, binary.BigEndian, height); err != nil {
		return nil, err
	}
	return key.Bytes(), nil
}

func computeChainTipKey() ([]byte, error) {
	key := new(bytes.Buffer)
	if err := key.WriteByte(chainTipPrefix); err != nil {
		return nil, err
	}
	return key.Bytes(), nil
}

func computeTransactionIndexKey(id TransactionID) ([]byte, error) {
	key := new(bytes.Buffer)
	if err := key.WriteByte(transactionIndexPrefix); err != nil {
		return nil, err
	}
	if err := binary.Write(key, binary.BigEndian, id[:]); err != nil {
		return nil, err
	}
	return key.Bytes(), nil
}

func computePubKeyTransactionIndexKey(
	pubKey ed25519.PublicKey, height *int64, index *int) ([]byte, error) {
	key := new(bytes.Buffer)
	if err := key.WriteByte(pubKeyTransactionIndexPrefix); err != nil {
		return nil, err
	}
	if err := binary.Write(key, binary.BigEndian, pubKey); err != nil {
		return nil, err
	}
	if height == nil {
		return key.Bytes(), nil
	}
	if err := binary.Write(key, binary.BigEndian, *height); err != nil {
		return nil, err
	}
	if index == nil {
		return key.Bytes(), nil
	}
	index32 := int32(*index)
	if err := binary.Write(key, binary.BigEndian, index32); err != nil {
		return nil, err
	}
	return key.Bytes(), nil
}

func decodePubKeyTransactionIndexKey(key []byte) (ed25519.PublicKey, int64, int, error) {
	buf := bytes.NewBuffer(key)
	if _, err := buf.ReadByte(); err != nil {
		return nil, 0, 0, err
	}
	var pubKey [ed25519.PublicKeySize]byte
	if err := binary.Read(buf, binary.BigEndian, pubKey[:32]); err != nil {
		return nil, 0, 0, err
	}
	var height int64
	if err := binary.Read(buf, binary.BigEndian, &height); err != nil {
		return nil, 0, 0, err
	}
	var index int32
	if err := binary.Read(buf, binary.BigEndian, &index); err != nil {
		return nil, 0, 0, err
	}
	return ed25519.PublicKey(pubKey[:]), height, int(index), nil
}

func computePubKeyBalanceKey(pubKey ed25519.PublicKey) ([]byte, error) {
	key := new(bytes.Buffer)
	if err := key.WriteByte(pubKeyBalancePrefix); err != nil {
		return nil, err
	}
	if err := binary.Write(key, binary.BigEndian, pubKey); err != nil {
		return nil, err
	}
	return key.Bytes(), nil
}

func encodeChainTip(id BlockID, height int64) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, id); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, height); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeChainTip(ctBytes []byte) (*BlockID, int64, error) {
	buf := bytes.NewBuffer(ctBytes)
	id := new(BlockID)
	if err := binary.Read(buf, binary.BigEndian, id); err != nil {
		return nil, 0, err
	}
	var height int64
	if err := binary.Read(buf, binary.BigEndian, &height); err != nil {
		return nil, 0, err
	}
	return id, height, nil
}

func encodeNumber(num int64) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, num); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeTransactionIndex(height int64, index int) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, height); err != nil {
		return nil, err
	}
	index32 := int32(index)
	if err := binary.Write(buf, binary.BigEndian, index32); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeTransactionIndex(indexBytes []byte) (int64, int, error) {
	buf := bytes.NewBuffer(indexBytes)
	var height int64
	if err := binary.Read(buf, binary.BigEndian, &height); err != nil {
		return 0, 0, err
	}
	var index int32
	if err := binary.Read(buf, binary.BigEndian, &index); err != nil {
		return 0, 0, err
	}
	return height, int(index), nil
}
