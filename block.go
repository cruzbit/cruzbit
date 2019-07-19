// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"math/big"
	"math/rand"
	"time"

	"golang.org/x/crypto/sha3"
)

// Block represents a block in the block chain. It has a header and a list of transactions.
// As blocks are connected their transactions affect the underlying ledger.
type Block struct {
	Header       *BlockHeader   `json:"header"`
	Transactions []*Transaction `json:"transactions"`
	hasher       hash.Hash      // hash state used by miner. not marshaled
}

// BlockHeader contains data used to determine block validity and its place in the block chain.
type BlockHeader struct {
	Previous         BlockID            `json:"previous"`
	HashListRoot     TransactionID      `json:"hash_list_root"`
	Time             int64              `json:"time"`
	Target           BlockID            `json:"target"`
	ChainWork        BlockID            `json:"chain_work"` // total cumulative chain work
	Nonce            int64              `json:"nonce"`      // not used for crypto
	Height           int64              `json:"height"`
	TransactionCount int32              `json:"transaction_count"`
	hasher           *BlockHeaderHasher // used to speed up mining. not marshaled
}

// BlockID is a block's unique identifier.
type BlockID [32]byte // SHA3-256 hash

// NewBlock creates and returns a new Block to be mined.
func NewBlock(previous BlockID, height int64, target, chainWork BlockID, transactions []*Transaction) (
	*Block, error) {

	// enforce the hard cap transaction limit
	if len(transactions) > MAX_TRANSACTIONS_PER_BLOCK {
		return nil, fmt.Errorf("Transaction list size exceeds limit per block")
	}

	// compute the hash list root
	hasher := sha3.New256()
	hashListRoot, err := computeHashListRoot(hasher, transactions)
	if err != nil {
		return nil, err
	}

	// create the header and block
	return &Block{
		Header: &BlockHeader{
			Previous:         previous,
			HashListRoot:     hashListRoot,
			Time:             time.Now().Unix(), // just use the system time
			Target:           target,
			ChainWork:        computeChainWork(target, chainWork),
			Nonce:            rand.Int63n(MAX_NUMBER),
			Height:           height,
			TransactionCount: int32(len(transactions)),
		},
		Transactions: transactions,
		hasher:       hasher, // save this to use while mining
	}, nil
}

// ID computes an ID for a given block.
func (b Block) ID() (BlockID, error) {
	return b.Header.ID()
}

// CheckPOW verifies the block's proof-of-work satisfies the declared target.
func (b Block) CheckPOW(id BlockID) bool {
	return id.GetBigInt().Cmp(b.Header.Target.GetBigInt()) <= 0
}

// AddTransaction adds a new transaction to the block. Called by miner when mining a new block.
func (b *Block) AddTransaction(id TransactionID, tx *Transaction) error {
	// hash the new transaction hash with the running state
	b.hasher.Write(id[:])

	// update coinbase's fee
	b.Transactions[0].Amount += tx.Fee

	// update the hash list root to account for coinbase amount change
	var err error
	b.Header.HashListRoot, err = addCoinbaseToHashListRoot(b.hasher, b.Transactions[0])
	if err != nil {
		return err
	}

	// append the new transaction to the list
	b.Transactions = append(b.Transactions, tx)
	b.Header.TransactionCount += 1
	return nil
}

// Compute a hash list root of all transaction hashes
func computeHashListRoot(hasher hash.Hash, transactions []*Transaction) (TransactionID, error) {
	if hasher == nil {
		hasher = sha3.New256()
	}

	// don't include coinbase in the first round
	for _, tx := range transactions[1:] {
		id, err := tx.ID()
		if err != nil {
			return TransactionID{}, err
		}
		hasher.Write(id[:])
	}

	// add the coinbase last
	return addCoinbaseToHashListRoot(hasher, transactions[0])
}

// Add the coinbase to the hash list root
func addCoinbaseToHashListRoot(hasher hash.Hash, coinbase *Transaction) (TransactionID, error) {
	// get the root of all of the non-coinbase transaction hashes
	rootHashWithoutCoinbase := hasher.Sum(nil)

	// add the coinbase separately
	// this makes adding new transactions while mining more efficient since the coinbase
	// fee amount will change when adding new transactions to the block
	id, err := coinbase.ID()
	if err != nil {
		return TransactionID{}, err
	}

	// hash the coinbase hash with the transaction list root hash
	rootHash := sha3.New256()
	rootHash.Write(id[:])
	rootHash.Write(rootHashWithoutCoinbase[:])

	// we end up with a sort of modified hash list root of the form:
	// HashListRoot = H(TXID[0] | H(TXID[1] | ... | TXID[N-1]))
	var hashListRoot TransactionID
	copy(hashListRoot[:], rootHash.Sum(nil))
	return hashListRoot, nil
}

// Compute block work given its target
func computeBlockWork(target BlockID) *big.Int {
	blockWorkInt := big.NewInt(0)
	targetInt := target.GetBigInt()
	if targetInt.Cmp(blockWorkInt) <= 0 {
		return blockWorkInt
	}
	// block work = 2**256 / (target+1)
	maxInt := new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil)
	targetInt.Add(targetInt, big.NewInt(1))
	return blockWorkInt.Div(maxInt, targetInt)
}

// Compute cumulative chain work given a block's target and the previous chain work
func computeChainWork(target, chainWork BlockID) (newChainWork BlockID) {
	blockWorkInt := computeBlockWork(target)
	chainWorkInt := chainWork.GetBigInt()
	chainWorkInt = chainWorkInt.Add(chainWorkInt, blockWorkInt)
	newChainWork.SetBigInt(chainWorkInt)
	return
}

// ID computes an ID for a given block header.
func (header BlockHeader) ID() (BlockID, error) {
	headerJson, err := json.Marshal(header)
	if err != nil {
		return BlockID{}, err
	}
	return sha3.Sum256([]byte(headerJson)), nil
}

// IDFast computes an ID for a given block header when mining.
func (header *BlockHeader) IDFast(minerNum int) (*big.Int, int64) {
	if header.hasher == nil {
		header.hasher = NewBlockHeaderHasher()
	}
	return header.hasher.Update(minerNum, header)
}

// Compare returns true if the header indicates it is a better chain than "theirHeader" up to both points.
// "thisWhen" is the timestamp of when we stored this block header.
// "theirWhen" is the timestamp of when we stored "theirHeader".
func (header BlockHeader) Compare(theirHeader *BlockHeader, thisWhen, theirWhen int64) bool {
	thisWorkInt := header.ChainWork.GetBigInt()
	theirWorkInt := theirHeader.ChainWork.GetBigInt()

	// most work wins
	if thisWorkInt.Cmp(theirWorkInt) > 0 {
		return true
	}
	if thisWorkInt.Cmp(theirWorkInt) < 0 {
		return false
	}

	// tie goes to the block we stored first
	if thisWhen < theirWhen {
		return true
	}
	if thisWhen > theirWhen {
		return false
	}

	// if we still need to break a tie go by the lesser id
	thisID, err := header.ID()
	if err != nil {
		panic(err)
	}
	theirID, err := theirHeader.ID()
	if err != nil {
		panic(err)
	}
	return thisID.GetBigInt().Cmp(theirID.GetBigInt()) < 0
}

// String implements the Stringer interface
func (id BlockID) String() string {
	return hex.EncodeToString(id[:])
}

// MarshalJSON marshals BlockID as a hex string.
func (id BlockID) MarshalJSON() ([]byte, error) {
	s := "\"" + id.String() + "\""
	return []byte(s), nil
}

// UnmarshalJSON unmarshals BlockID hex string to BlockID.
func (id *BlockID) UnmarshalJSON(b []byte) error {
	if len(b) != 64+2 {
		return fmt.Errorf("Invalid block ID")
	}
	idBytes, err := hex.DecodeString(string(b[1 : len(b)-1]))
	if err != nil {
		return err
	}
	copy(id[:], idBytes)
	return nil
}

// SetBigInt converts from big.Int to BlockID.
func (id *BlockID) SetBigInt(i *big.Int) *BlockID {
	intBytes := i.Bytes()
	if len(intBytes) > 32 {
		panic("Too much work")
	}
	for i := 0; i < len(id); i++ {
		id[i] = 0x00
	}
	copy(id[32-len(intBytes):], intBytes)
	return id
}

// GetBigInt converts from BlockID to big.Int.
func (id BlockID) GetBigInt() *big.Int {
	return new(big.Int).SetBytes(id[:])
}
