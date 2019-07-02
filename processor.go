// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"sort"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/ed25519"
)

// Processor processes blocks and transactions in order to construct the ledger.
// It also manages the storage of all block chain data as well as inclusion of new transactions into the transaction queue.
type Processor struct {
	genesisID               BlockID
	blockStore              BlockStorage                  // storage of raw block data
	txQueue                 TransactionQueue              // queue of transactions to confirm
	ledger                  Ledger                        // ledger built from processing blocks
	txChan                  chan txToProcess              // receive new transactions to process on this channel
	blockChan               chan blockToProcess           // receive new blocks to process on this channel
	registerNewTxChan       chan chan<- NewTx             // receive registration requests for new transaction notifications
	unregisterNewTxChan     chan chan<- NewTx             // receive unregistration requests for new transaction notifications
	registerTipChangeChan   chan chan<- TipChange         // receive registration requests for tip change notifications
	unregisterTipChangeChan chan chan<- TipChange         // receive unregistration requests for tip change notifications
	newTxChannels           map[chan<- NewTx]struct{}     // channels needing notification of newly processed transactions
	tipChangeChannels       map[chan<- TipChange]struct{} // channels needing notification of changes to main chain tip blocks
	shutdownChan            chan struct{}
	wg                      sync.WaitGroup
}

// NewTx is a message sent to registered new transaction channels when a transaction is queued.
type NewTx struct {
	TransactionID TransactionID // transaction ID
	Transaction   *Transaction  // new transaction
	Source        string        // who sent it
}

// TipChange is a message sent to registered new tip channels on main chain tip (dis-)connection..
type TipChange struct {
	BlockID BlockID // block ID of the main chain tip block
	Block   *Block  // full block
	Source  string  // who sent the block that caused this change
	Connect bool    // true if the tip has been connected. false for disconnected
	More    bool    // true if the tip has been connected and more connections are expected
}

type txToProcess struct {
	id         TransactionID // transaction ID
	tx         *Transaction  // transaction to process
	source     string        // who sent it
	resultChan chan<- error  // channel to receive the result
}

type blockToProcess struct {
	id         BlockID      // block ID
	block      *Block       // block to process
	source     string       // who sent it
	resultChan chan<- error // channel to receive the result
}

// NewProcessor returns a new Processor instance.
func NewProcessor(genesisID BlockID, blockStore BlockStorage, txQueue TransactionQueue, ledger Ledger) *Processor {
	return &Processor{
		genesisID:               genesisID,
		blockStore:              blockStore,
		txQueue:                 txQueue,
		ledger:                  ledger,
		txChan:                  make(chan txToProcess, 100),
		blockChan:               make(chan blockToProcess, 10),
		registerNewTxChan:       make(chan chan<- NewTx),
		unregisterNewTxChan:     make(chan chan<- NewTx),
		registerTipChangeChan:   make(chan chan<- TipChange),
		unregisterTipChangeChan: make(chan chan<- TipChange),
		newTxChannels:           make(map[chan<- NewTx]struct{}),
		tipChangeChannels:       make(map[chan<- TipChange]struct{}),
		shutdownChan:            make(chan struct{}),
	}
}

// Run executes the Processor's main loop in its own goroutine.
// It verifies and processes blocks and transactions.
func (p *Processor) Run() {
	p.wg.Add(1)
	go p.run()
}

func (p *Processor) run() {
	defer p.wg.Done()

	for {
		select {
		case txToProcess := <-p.txChan:
			// process a transaction
			err := p.processTransaction(txToProcess.id, txToProcess.tx, txToProcess.source)
			if err != nil {
				log.Println(err)
			}

			// send back the result
			txToProcess.resultChan <- err

		case blockToProcess := <-p.blockChan:
			// process a block
			before := time.Now().UnixNano()
			err := p.processBlock(blockToProcess.id, blockToProcess.block, blockToProcess.source)
			if err != nil {
				log.Println(err)
			}
			after := time.Now().UnixNano()

			log.Printf("Processing took %d ms, %d transaction(s), transaction queue length: %d\n",
				(after-before)/int64(time.Millisecond),
				len(blockToProcess.block.Transactions),
				p.txQueue.Len())

			// send back the result
			blockToProcess.resultChan <- err

		case ch := <-p.registerNewTxChan:
			p.newTxChannels[ch] = struct{}{}

		case ch := <-p.unregisterNewTxChan:
			delete(p.newTxChannels, ch)

		case ch := <-p.registerTipChangeChan:
			p.tipChangeChannels[ch] = struct{}{}

		case ch := <-p.unregisterTipChangeChan:
			delete(p.tipChangeChannels, ch)

		case _, ok := <-p.shutdownChan:
			if !ok {
				log.Println("Processor shutting down...")
				return
			}
		}
	}
}

// ProcessTransaction is called to process a new candidate transaction for the transaction queue.
func (p *Processor) ProcessTransaction(id TransactionID, tx *Transaction, from string) error {
	resultChan := make(chan error)
	p.txChan <- txToProcess{id: id, tx: tx, source: from, resultChan: resultChan}
	return <-resultChan
}

// ProcessBlock is called to process a new candidate block chain tip.
func (p *Processor) ProcessBlock(id BlockID, block *Block, from string) error {
	resultChan := make(chan error)
	p.blockChan <- blockToProcess{id: id, block: block, source: from, resultChan: resultChan}
	return <-resultChan
}

// RegisterForNewTransactions is called to register to receive notifications of newly queued transactions.
func (p *Processor) RegisterForNewTransactions(ch chan<- NewTx) {
	p.registerNewTxChan <- ch
}

// UnregisterForNewTransactions is called to unregister to receive notifications of newly queued transactions
func (p *Processor) UnregisterForNewTransactions(ch chan<- NewTx) {
	p.unregisterNewTxChan <- ch
}

// RegisterForTipChange is called to register to receive notifications of tip block changes.
func (p *Processor) RegisterForTipChange(ch chan<- TipChange) {
	p.registerTipChangeChan <- ch
}

// UnregisterForTipChange is called to unregister to receive notifications of tip block changes.
func (p *Processor) UnregisterForTipChange(ch chan<- TipChange) {
	p.unregisterTipChangeChan <- ch
}

// Shutdown stops the processor synchronously.
func (p *Processor) Shutdown() {
	close(p.shutdownChan)
	p.wg.Wait()
	log.Println("Processor shutdown")
}

// Process a transaction
func (p *Processor) processTransaction(id TransactionID, tx *Transaction, source string) error {
	log.Printf("Processing transaction %s\n", id)

	// min fee? if not waste no more time
	if tx.Fee < MIN_FEE_CRUZBITS {
		return fmt.Errorf("Transaction %s doesn't pay minimum fee %.6f\n",
			id, float64(MIN_FEE_CRUZBITS)/CRUZBITS_PER_CRUZ)
	}

	// min amount? if not waste no more time
	if tx.Amount < MIN_AMOUNT_CRUZBITS {
		return fmt.Errorf("Transaction %s amount too small, minimum is %.6f\n",
			id, float64(MIN_AMOUNT_CRUZBITS)/CRUZBITS_PER_CRUZ)
	}

	// context-free checks
	if err := checkTransaction(id, tx); err != nil {
		return err
	}

	// no loose coinbases
	if tx.IsCoinbase() {
		return fmt.Errorf("Coinbase transaction %s only allowed in block", id)
	}

	// is the queue full?
	if p.txQueue.Len() >= MAX_TRANSACTION_QUEUE_LENGTH {
		return fmt.Errorf("No room for transaction %s, queue is full", id)
	}

	// is it confirmed already?
	blockID, _, err := p.ledger.GetTransactionIndex(id)
	if err != nil {
		return err
	}
	if blockID != nil {
		return fmt.Errorf("Transaction %s is already confirmed", id)
	}

	// check series, maturity and expiration
	tipID, tipHeight, err := p.ledger.GetChainTip()
	if err != nil {
		return err
	}
	if tipID == nil {
		return fmt.Errorf("No main chain tip id found")
	}

	// is the series current for inclusion in the next block?
	if !checkTransactionSeries(tx, tipHeight+1) {
		return fmt.Errorf("Transaction %s would have invalid series", id)
	}

	// would it be mature if included in the next block?
	if !tx.IsMature(tipHeight + 1) {
		return fmt.Errorf("Transaction %s would not be mature", id)
	}

	// is it expired if included in the next block?
	if tx.IsExpired(tipHeight + 1) {
		return fmt.Errorf("Transaction %s is expired, height: %d, expires: %d",
			id, tipHeight, tx.Expires)
	}

	// verify signature
	ok, err := tx.Verify()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("Signature verification failed for %s", id)
	}

	// rejects a transaction if sender would have insufficient balance
	ok, err = p.txQueue.Add(id, tx)
	if err != nil {
		return err
	}
	if !ok {
		// don't notify others if the transaction already exists in the queue
		return nil
	}

	// notify channels
	for ch := range p.newTxChannels {
		ch <- NewTx{TransactionID: id, Transaction: tx, Source: source}
	}
	return nil
}

// Context-free transaction sanity checker
func checkTransaction(id TransactionID, tx *Transaction) error {
	// sane-ish time.
	// transaction timestamps are strictly for user and application usage.
	// we make no claims to their validity and rely on them for nothing.
	if tx.Time < 0 || tx.Time > MAX_NUMBER {
		return fmt.Errorf("Invalid transaction time, transaction: %s", id)
	}

	// no negative nonces
	if tx.Nonce < 0 {
		return fmt.Errorf("Negative nonce value, transaction: %s", id)
	}

	if tx.IsCoinbase() {
		// no fee in coinbase
		if tx.Fee > 0 {
			return fmt.Errorf("Coinbase can't have a fee, transaction: %s", id)
		}
		// no maturity for coinbase
		if tx.Matures > 0 {
			return fmt.Errorf("Coinbase can't have a maturity, transaction: %s", id)
		}
		// no expiration for coinbase
		if tx.Expires > 0 {
			return fmt.Errorf("Coinbase can't expire, transaction: %s", id)
		}
		// no signature on coinbase
		if len(tx.Signature) != 0 {
			return fmt.Errorf("Coinbase can't have a signature, transaction: %s", id)
		}
	} else {
		// sanity check sender
		if len(tx.From) != ed25519.PublicKeySize {
			return fmt.Errorf("Invalid transaction sender, transaction: %s", id)
		}
		// sanity check signature
		if len(tx.Signature) != ed25519.SignatureSize {
			return fmt.Errorf("Invalid transaction signature, transaction: %s", id)
		}
	}

	// sanity check recipient
	if tx.To == nil {
		return fmt.Errorf("Transaction %s missing recipient", id)
	}
	if len(tx.To) != ed25519.PublicKeySize {
		return fmt.Errorf("Invalid transaction recipient, transaction: %s", id)
	}

	// no pays to self
	if bytes.Equal(tx.From, tx.To) {
		return fmt.Errorf("Transaction %s to self is invalid", id)
	}

	// sanity check amount and fee
	if tx.Amount <= 0 {
		return fmt.Errorf("Transaction %s contains invalid amount", id)
	}
	if tx.Amount > MAX_MONEY {
		return fmt.Errorf("Transaction %s contains too large of an amount", id)
	}
	if tx.Fee < 0 {
		return fmt.Errorf("Transaction %s contains negative fee", id)
	}
	if tx.Fee > MAX_MONEY {
		return fmt.Errorf("Transaction %s contains too large of a fee", id)
	}

	// make sure memo is valid ascii/utf8
	if !utf8.ValidString(tx.Memo) {
		return fmt.Errorf("Transaction %s memo contains invalid utf8 characters", id)
	}

	// check memo length
	if len(tx.Memo) > MAX_MEMO_LENGTH {
		return fmt.Errorf("Transaction %s memo length exceeded", id)
	}

	// sanity check maturity, expiration and series
	if tx.Matures < 0 || tx.Matures > MAX_NUMBER {
		return fmt.Errorf("Invalid maturity, transaction: %s", id)
	}
	if tx.Expires < 0 || tx.Expires > MAX_NUMBER {
		return fmt.Errorf("Invalid expiration, transaction: %s", id)
	}
	if tx.Series <= 0 || tx.Series > MAX_NUMBER {
		return fmt.Errorf("Invalid series, transaction: %s", id)
	}

	return nil
}

// The series must be within the acceptable range given the current height
func checkTransactionSeries(tx *Transaction, height int64) bool {
	if tx.From == nil {
		// coinbases must start a new series right on time
		return tx.Series == height/BLOCKS_UNTIL_NEW_SERIES+1
	}

	// user transactions have a grace period (1 full series) to mitigate effects
	// of any potential queueing delay and/or reorgs near series switchover time
	high := height/BLOCKS_UNTIL_NEW_SERIES + 1
	low := high - 1
	if low == 0 {
		low = 1
	}
	return tx.Series >= low && tx.Series <= high
}

// Process a block
func (p *Processor) processBlock(id BlockID, block *Block, source string) error {
	log.Printf("Processing block %s\n", id)

	now := time.Now().Unix()

	// did we process this block already?
	branchType, err := p.ledger.GetBranchType(id)
	if err != nil {
		return err
	}
	if branchType != UNKNOWN {
		log.Printf("Already processed block %s", id)
		return nil
	}

	// sanity check the block
	if err := checkBlock(id, block, now); err != nil {
		return err
	}

	// have we processed its parent?
	branchType, err = p.ledger.GetBranchType(block.Header.Previous)
	if err != nil {
		return err
	}
	if branchType != MAIN && branchType != SIDE {
		if id == p.genesisID {
			// store it
			if err := p.blockStore.Store(id, block, now); err != nil {
				return err
			}
			// begin the ledger
			if err := p.connectBlock(id, block, source, false); err != nil {
				return err
			}
			log.Printf("Connected block %s\n", id)
			return nil
		}
		// current block is an orphan
		return fmt.Errorf("Block %s is an orphan", id)
	}

	// attempt to extend the chain
	return p.acceptBlock(id, block, now, source)
}

// Context-free block sanity checker
func checkBlock(id BlockID, block *Block, now int64) error {
	// sanity check time
	if block.Header.Time < 0 || block.Header.Time > MAX_NUMBER {
		return fmt.Errorf("Time value is invalid, block %s", id)
	}

	// check timestamp isn't too far in the future
	if block.Header.Time > now+MAX_FUTURE_SECONDS {
		return fmt.Errorf(
			"Timestamp %d too far in the future, now %d, block %s",
			block.Header.Time,
			now,
			id,
		)
	}

	// proof-of-work should satisfy declared target
	if !block.CheckPOW(id) {
		return fmt.Errorf("Insufficient proof-of-work for block %s", id)
	}

	// sanity check nonce
	if block.Header.Nonce < 0 || block.Header.Nonce > MAX_NUMBER {
		return fmt.Errorf("Nonce value is invalid, block %s", id)
	}

	// sanity check height
	if block.Header.Height < 0 || block.Header.Height > MAX_NUMBER {
		return fmt.Errorf("Height value is invalid, block %s", id)
	}

	// sanity check transaction count
	if block.Header.TransactionCount < 0 {
		return fmt.Errorf("Negative transaction count in header of block %s", id)
	}

	if int(block.Header.TransactionCount) != len(block.Transactions) {
		return fmt.Errorf("Transaction count in header doesn't match block %s", id)
	}

	// must have at least one transaction
	if len(block.Transactions) == 0 {
		return fmt.Errorf("No transactions in block %s", id)
	}

	// first tx must be a coinbase
	if !block.Transactions[0].IsCoinbase() {
		return fmt.Errorf("First transaction is not a coinbase in block %s", id)
	}

	// check max number of transactions
	max := computeMaxTransactionsPerBlock(block.Header.Height)
	if len(block.Transactions) > max {
		return fmt.Errorf("Block %s contains too many transactions %d, max: %d",
			id, len(block.Transactions), max)
	}

	// the rest must not be coinbases
	if len(block.Transactions) > 1 {
		for i := 1; i < len(block.Transactions); i++ {
			if block.Transactions[i].IsCoinbase() {
				return fmt.Errorf("Multiple coinbase transactions in block %s", id)
			}
		}
	}

	// basic transaction checks that don't depend on context
	txIDs := make(map[TransactionID]bool)
	for _, tx := range block.Transactions {
		id, err := tx.ID()
		if err != nil {
			return err
		}
		if err := checkTransaction(id, tx); err != nil {
			return err
		}
		txIDs[id] = true
	}

	// check for duplicate transactions
	if len(txIDs) != len(block.Transactions) {
		return fmt.Errorf("Duplicate transaction in block %s", id)
	}

	// verify hash list root
	hashListRoot, err := computeHashListRoot(nil, block.Transactions)
	if err != nil {
		return err
	}
	if hashListRoot != block.Header.HashListRoot {
		return fmt.Errorf("Hash list root mismatch for block %s", id)
	}

	return nil
}

// Computes the maximum number of transactions allowed in a block at the given height. Inspired by BIP 101
func computeMaxTransactionsPerBlock(height int64) int {
	if height >= MAX_TRANSACTIONS_PER_BLOCK_EXCEEDED_AT_HEIGHT {
		// I guess we can revisit this sometime in the next 35 years if necessary
		return MAX_TRANSACTIONS_PER_BLOCK
	}

	// piecewise-linear-between-doublings growth
	doublings := height / BLOCKS_UNTIL_TRANSACTIONS_PER_BLOCK_DOUBLING
	if doublings >= 64 {
		panic("Overflow uint64")
	}
	remainder := height % BLOCKS_UNTIL_TRANSACTIONS_PER_BLOCK_DOUBLING
	factor := int64(1 << uint64(doublings))
	interpolate := (INITIAL_MAX_TRANSACTIONS_PER_BLOCK * factor * remainder) /
		BLOCKS_UNTIL_TRANSACTIONS_PER_BLOCK_DOUBLING
	return int(INITIAL_MAX_TRANSACTIONS_PER_BLOCK*factor + interpolate)
}

// Attempt to extend the chain with the new block
func (p *Processor) acceptBlock(id BlockID, block *Block, now int64, source string) error {
	prevHeader, _, err := p.blockStore.GetBlockHeader(block.Header.Previous)
	if err != nil {
		return err
	}

	// check height
	newHeight := prevHeader.Height + 1
	if block.Header.Height != newHeight {
		return fmt.Errorf("Expected height %d found %d for block %s",
			newHeight, block.Header.Height, id)
	}

	// did we process it already?
	branchType, err := p.ledger.GetBranchType(id)
	if err != nil {
		return err
	}
	if branchType != UNKNOWN {
		log.Printf("Already processed block %s", id)
		return nil
	}

	// check declared proof of work is correct
	target, err := computeTarget(prevHeader, p.blockStore)
	if err != nil {
		return err
	}
	if block.Header.Target != target {
		return fmt.Errorf("Incorrect target %s, expected %s for block %s",
			block.Header.Target, target, id)
	}

	// check that cumulative work is correct
	chainWork := computeChainWork(block.Header.Target, prevHeader.ChainWork)
	if block.Header.ChainWork != chainWork {
		return fmt.Errorf("Incorrect chain work %s, expected %s for block %s",
			block.Header.ChainWork, chainWork, id)
	}

	// check that the timestamp isn't too far in the past
	medianTimestamp, err := computeMedianTimestamp(prevHeader, p.blockStore)
	if err != nil {
		return err
	}
	if block.Header.Time <= medianTimestamp {
		return fmt.Errorf("Timestamp is too early for block %s", id)
	}

	// check series, maturity, expiration then verify signatures and calculate total fees
	var fees int64
	for _, tx := range block.Transactions {
		txID, err := tx.ID()
		if err != nil {
			return err
		}
		if !checkTransactionSeries(tx, block.Header.Height) {
			return fmt.Errorf("Transaction %s would have invalid series", txID)
		}
		if !tx.IsCoinbase() {
			if !tx.IsMature(block.Header.Height) {
				return fmt.Errorf("Transaction %s is immature", txID)
			}
			if tx.IsExpired(block.Header.Height) {
				return fmt.Errorf("Transaction %s is expired", txID)
			}
			// if it's in the queue with the same signature we've verified it already
			if !p.txQueue.ExistsSigned(txID, tx.Signature) {
				ok, err := tx.Verify()
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("Signature verification failed, transaction: %s", txID)
				}
			}
		}
		fees += tx.Fee
	}

	// verify coinbase reward
	reward := BlockCreationReward(block.Header.Height) + fees
	if block.Transactions[0].Amount != reward {
		// in cruzbit every last issued bit must be accounted for in public key balances
		return fmt.Errorf("Coinbase pays incorrect amount, block %s", id)
	}

	// store the block if we think we're going to accept it
	if err := p.blockStore.Store(id, block, now); err != nil {
		return err
	}

	// get the current tip before we try adjusting the chain
	tipID, _, err := p.ledger.GetChainTip()
	if err != nil {
		return err
	}

	// finish accepting the block if possible
	if err := p.acceptBlockContinue(id, block, now, prevHeader, source); err != nil {
		// we may have disconnected the old best chain and partially
		// connected the new one before encountering a problem. re-activate it now
		if err2 := p.reconnectTip(*tipID, source); err2 != nil {
			log.Printf("Error reconnecting tip: %s, block: %s\n", err2, *tipID)
		}
		// return the original error
		return err
	}

	return nil
}

// BlockCreationReward computes the expected block reward for the given height.
func BlockCreationReward(height int64) int64 {
	halvings := height / BLOCKS_UNTIL_REWARD_HALVING
	if halvings >= 64 {
		return 0
	}
	var reward int64 = INITIAL_COINBASE_REWARD
	reward >>= uint64(halvings)
	return reward
}

// Compute expected target of the current block
func computeTarget(prevHeader *BlockHeader, blockStore BlockStorage) (BlockID, error) {
	if (prevHeader.Height+1)%RETARGET_INTERVAL != 0 {
		// not 2016th block, use previous block's value
		return prevHeader.Target, nil
	}

	// defend against time warp attack
	blocksToGoBack := RETARGET_INTERVAL - 1
	if (prevHeader.Height + 1) != RETARGET_INTERVAL {
		blocksToGoBack = RETARGET_INTERVAL
	}

	// walk back to the first block of the interval
	firstHeader := prevHeader
	for i := 0; i < blocksToGoBack; i++ {
		var err error
		firstHeader, _, err = blockStore.GetBlockHeader(firstHeader.Previous)
		if err != nil {
			return BlockID{}, err
		}
	}

	actualTimespan := prevHeader.Time - firstHeader.Time

	minTimespan := int64(RETARGET_TIME / 4)
	maxTimespan := int64(RETARGET_TIME * 4)

	if actualTimespan < minTimespan {
		actualTimespan = minTimespan
	}
	if actualTimespan > maxTimespan {
		actualTimespan = maxTimespan
	}

	actualTimespanInt := big.NewInt(actualTimespan)
	retargetTimeInt := big.NewInt(RETARGET_TIME)

	initialTargetBytes, err := hex.DecodeString(INITIAL_TARGET)
	if err != nil {
		return BlockID{}, err
	}

	maxTargetInt := new(big.Int).SetBytes(initialTargetBytes)
	prevTargetInt := new(big.Int).SetBytes(prevHeader.Target[:])
	newTargetInt := new(big.Int).Mul(prevTargetInt, actualTimespanInt)
	newTargetInt.Div(newTargetInt, retargetTimeInt)

	var target BlockID
	if newTargetInt.Cmp(maxTargetInt) > 0 {
		target.SetBigInt(maxTargetInt)
	} else {
		target.SetBigInt(newTargetInt)
	}

	return target, nil
}

// Compute the median timestamp of the last NUM_BLOCKS_FOR_MEDIAN_TIMESTAMP blocks
func computeMedianTimestamp(prevHeader *BlockHeader, blockStore BlockStorage) (int64, error) {
	var timestamps []int64
	var err error
	for i := 0; i < NUM_BLOCKS_FOR_MEDIAN_TMESTAMP; i++ {
		timestamps = append(timestamps, prevHeader.Time)
		prevHeader, _, err = blockStore.GetBlockHeader(prevHeader.Previous)
		if err != nil {
			return 0, err
		}
		if prevHeader == nil {
			break
		}
	}
	sort.Slice(timestamps, func(i, j int) bool {
		return timestamps[i] < timestamps[j]
	})
	return timestamps[len(timestamps)/2], nil
}

// Continue accepting the block
func (p *Processor) acceptBlockContinue(
	id BlockID, block *Block, blockWhen int64, prevHeader *BlockHeader, source string) error {

	// get the current tip
	tipID, tipHeader, tipWhen, err := getChainTipHeader(p.ledger, p.blockStore)
	if err != nil {
		return err
	}
	if id == *tipID {
		// can happen if we failed connecting a new block
		return nil
	}

	// is this block better than the current tip?
	if !block.Header.Compare(tipHeader, blockWhen, tipWhen) {
		// flag this as a side branch block
		log.Printf("Block %s does not represent the tip of the best chain", id)
		return p.ledger.SetBranchType(id, SIDE)
	}

	// the new block is the better chain
	tipAncestor := tipHeader
	newAncestor := prevHeader

	minHeight := tipAncestor.Height
	if newAncestor.Height < minHeight {
		minHeight = newAncestor.Height
	}

	var blocksToDisconnect, blocksToConnect []BlockID

	// walk back each chain to the common minHeight
	tipAncestorID := *tipID
	for tipAncestor.Height > minHeight {
		blocksToDisconnect = append(blocksToDisconnect, tipAncestorID)
		tipAncestorID = tipAncestor.Previous
		tipAncestor, _, err = p.blockStore.GetBlockHeader(tipAncestorID)
		if err != nil {
			return err
		}
	}

	newAncestorID := block.Header.Previous
	for newAncestor.Height > minHeight {
		blocksToConnect = append([]BlockID{newAncestorID}, blocksToConnect...)
		newAncestorID = newAncestor.Previous
		newAncestor, _, err = p.blockStore.GetBlockHeader(newAncestorID)
		if err != nil {
			return err
		}
	}

	// scan both chains until we get to the common ancestor
	for *newAncestor != *tipAncestor {
		blocksToDisconnect = append(blocksToDisconnect, tipAncestorID)
		blocksToConnect = append([]BlockID{newAncestorID}, blocksToConnect...)
		tipAncestorID = tipAncestor.Previous
		tipAncestor, _, err = p.blockStore.GetBlockHeader(tipAncestorID)
		if err != nil {
			return err
		}
		newAncestorID = newAncestor.Previous
		newAncestor, _, err = p.blockStore.GetBlockHeader(newAncestorID)
		if err != nil {
			return err
		}
	}

	// we're at common ancestor. disconnect any main chain blocks we need to
	for _, id := range blocksToDisconnect {
		blockToDisconnect, err := p.blockStore.GetBlock(id)
		if err != nil {
			return err
		}
		if err := p.disconnectBlock(id, blockToDisconnect, source); err != nil {
			return err
		}
	}

	// connect any new chain blocks we need to
	for _, id := range blocksToConnect {
		blockToConnect, err := p.blockStore.GetBlock(id)
		if err != nil {
			return err
		}
		if err := p.connectBlock(id, blockToConnect, source, true); err != nil {
			return err
		}
	}

	// and finally connect the new block
	return p.connectBlock(id, block, source, false)
}

// Update the ledger and transaction queue and notify undo tip channels
func (p *Processor) disconnectBlock(id BlockID, block *Block, source string) error {
	// Update the ledger
	txIDs, err := p.ledger.DisconnectBlock(id, block)
	if err != nil {
		return err
	}

	log.Printf("Block %s has been disconnected, height: %d\n", id, block.Header.Height)

	// Add newly disconnected non-coinbase transactions back to the queue
	if err := p.txQueue.AddBatch(txIDs[1:], block.Transactions[1:], block.Header.Height-1); err != nil {
		return err
	}

	// Notify tip change channels
	for ch := range p.tipChangeChannels {
		ch <- TipChange{BlockID: id, Block: block, Source: source}
	}
	return nil
}

// Update the ledger and transaction queue and notify new tip channels
func (p *Processor) connectBlock(id BlockID, block *Block, source string, more bool) error {
	// Update the ledger
	txIDs, err := p.ledger.ConnectBlock(id, block)
	if err != nil {
		return err
	}

	log.Printf("Block %s is the new tip, height: %d\n", id, block.Header.Height)

	// Remove newly confirmed non-coinbase transactions from the queue
	if err := p.txQueue.RemoveBatch(txIDs[1:], block.Header.Height, more); err != nil {
		return err
	}

	// Notify tip change channels
	for ch := range p.tipChangeChannels {
		ch <- TipChange{BlockID: id, Block: block, Source: source, Connect: true, More: more}
	}
	return nil
}

// Try to reconnect the previous tip block when acceptBlockContinue fails for the new block
func (p *Processor) reconnectTip(id BlockID, source string) error {
	block, err := p.blockStore.GetBlock(id)
	if err != nil {
		return err
	}
	if block == nil {
		return fmt.Errorf("Block %s not found", id)
	}
	_, when, err := p.blockStore.GetBlockHeader(id)
	if err != nil {
		return err
	}
	prevHeader, _, err := p.blockStore.GetBlockHeader(block.Header.Previous)
	if err != nil {
		return err
	}
	return p.acceptBlockContinue(id, block, when, prevHeader, source)
}

// Convenience method to get the current main chain's tip ID, header, and storage time.
func getChainTipHeader(ledger Ledger, blockStore BlockStorage) (*BlockID, *BlockHeader, int64, error) {
	// get the current tip
	tipID, _, err := ledger.GetChainTip()
	if err != nil {
		return nil, nil, 0, err
	}
	if tipID == nil {
		return nil, nil, 0, nil
	}

	// get the header
	tipHeader, tipWhen, err := blockStore.GetBlockHeader(*tipID)
	if err != nil {
		return nil, nil, 0, err
	}
	return tipID, tipHeader, tipWhen, nil
}
