// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"log"
	"math/big"
	"math/rand"
	"sync"
	"time"

	"golang.org/x/crypto/ed25519"
)

// Miner tries to mine a new tip block.
type Miner struct {
	pubKeys      []ed25519.PublicKey // receipients of any block rewards we mine
	memo         string              // memo for coinbase of any blocks we mine
	blockStore   BlockStorage
	txQueue      TransactionQueue
	ledger       Ledger
	processor    *Processor
	num          int
	keyIndex     int
	shutdownChan chan struct{}
	wg           sync.WaitGroup
}

// NewMiner returns a new Miner instance.
func NewMiner(pubKeys []ed25519.PublicKey, memo string,
	blockStore BlockStorage, txQueue TransactionQueue,
	ledger Ledger, processor *Processor, num int) *Miner {
	return &Miner{
		pubKeys:      pubKeys,
		memo:         memo,
		blockStore:   blockStore,
		txQueue:      txQueue,
		ledger:       ledger,
		processor:    processor,
		num:          num,
		keyIndex:     rand.Intn(len(pubKeys)),
		shutdownChan: make(chan struct{}),
	}
}

// Run executes the miner's main loop in its own goroutine.
func (m *Miner) Run() {
	m.wg.Add(1)
	go m.run()
}

func (m *Miner) run() {
	defer m.wg.Done()

	tipChangeChan := make(chan TipChange, 1)
	m.processor.RegisterForTipChange(tipChangeChan)
	defer m.processor.UnregisterForTipChange(tipChangeChan)

	newTxChan := make(chan NewTx, 1)
	m.processor.RegisterForNewTransactions(newTxChan)
	defer m.processor.UnregisterForNewTransactions(newTxChan)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	var block *Block
	var targetInt *big.Int
	for {
		select {
		case tip := <-tipChangeChan:
			if !tip.Connect || tip.More {
				// only build off newly connected tip blocks
				continue
			}

			// give up whatever block we were working on
			log.Printf("Miner %d received notice of new tip block %s\n", m.num, tip.BlockID)

			var err error
			// start working on a new block
			block, err = m.createNextBlock(tip.BlockID, tip.Block.Header)
			if err != nil {
				// ledger state is broken
				panic(err)
			}
			targetInt = block.Header.Target.GetBigInt()

		case newTx := <-newTxChan:
			log.Printf("Miner %d received notice of new transaction %s\n", m.num, newTx.TransactionID)
			if block == nil {
				// we're not working on a block yet
				continue
			}

			if MAX_TRANSACTIONS_TO_INCLUDE_PER_BLOCK != 0 &&
				len(block.Transactions) >= MAX_TRANSACTIONS_TO_INCLUDE_PER_BLOCK {
				log.Printf("Per-block transaction limit hit (%d)\n", len(block.Transactions))
				continue
			}

			// add the transaction to the block (it updates the coinbase fee)
			if err := block.AddTransaction(newTx.TransactionID, newTx.Transaction); err != nil {
				log.Printf("Error adding new transaction %s to block: %s\n",
					newTx.TransactionID, err)
				// abandon the block
				block = nil
			}

		case _, ok := <-m.shutdownChan:
			if !ok {
				log.Printf("Miner %d shutting down...\n", m.num)
				return
			}

		case <-ticker.C:
			if block != nil {
				// update block time every so often
				block.Header.Time = time.Now().Unix()
			}

		default:
			if block == nil {
				// find the tip to start working off of
				tipID, tipHeader, _, err := getChainTipHeader(m.ledger, m.blockStore)
				if err != nil {
					panic(err)
				}
				// create a new block
				block, err = m.createNextBlock(*tipID, tipHeader)
				if err != nil {
					panic(err)
				}
				targetInt = block.Header.Target.GetBigInt()
			}

			// hash the block and check the proof-of-work
			idInt := block.Header.IDFast()
			if idInt.Cmp(targetInt) <= 0 {
				// found a solution
				id := new(BlockID).SetBigInt(idInt)
				log.Printf("Miner %d mined new block %s\n", m.num, *id)

				// process the block
				if err := m.processor.ProcessBlock(*id, block, "localhost"); err != nil {
					log.Printf("Error processing mined block: %s\n", err)
				}

				block = nil
				m.keyIndex = rand.Intn(len(m.pubKeys))
			} else {
				// no solution yet
				block.Header.Nonce += 1
				if block.Header.Nonce > MAX_NUMBER {
					block.Header.Nonce = 0
				}
			}
		}
	}
}

// Shutdown stops the miner synchronously.
func (m *Miner) Shutdown() {
	close(m.shutdownChan)
	m.wg.Wait()
	log.Printf("Miner %d shutdown\n", m.num)
}

// Create a new block off of the given tip block.
func (m *Miner) createNextBlock(tipID BlockID, tipHeader *BlockHeader) (*Block, error) {
	log.Printf("Miner %d mining new block from current tip %s\n", m.num, tipID)

	// fetch transactions to confirm from the queue
	txs := m.txQueue.Get(MAX_TRANSACTIONS_TO_INCLUDE_PER_BLOCK - 1)

	// calculate total fees
	var fees int64 = 0
	for _, tx := range txs {
		fees += tx.Fee
	}

	// calculate total block reward
	var newHeight int64 = tipHeader.Height + 1
	reward := BlockCreationReward(newHeight) + fees

	// build coinbase
	tx := NewTransaction(nil, m.pubKeys[m.keyIndex], reward, 0, 0, 0, newHeight, m.memo)

	// prepend coinbase
	txs = append([]*Transaction{tx}, txs...)

	// compute the next target
	newTarget, err := computeTarget(tipHeader, m.blockStore)
	if err != nil {
		return nil, err
	}

	// create the block
	block, err := NewBlock(tipID, newHeight, newTarget, tipHeader.ChainWork, txs)
	if err != nil {
		return nil, err
	}
	return block, nil
}
