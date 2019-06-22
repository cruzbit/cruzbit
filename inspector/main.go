// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	. "github.com/cruzbit/cruzbit"
	"github.com/logrusorgru/aurora"
	"golang.org/x/crypto/ed25519"
)

// A small tool to inspect the block chain and ledger offline
func main() {
	var commands = []string{
		"height", "balance", "balance_at", "block", "block_at", "tx", "history", "verify",
	}

	dataDirPtr := flag.String("datadir", "", "Path to a directory containing block chain data")
	pubKeyPtr := flag.String("pubkey", "", "Base64 encoded public key")
	cmdPtr := flag.String("command", "height", "Commands: "+strings.Join(commands, ", "))
	heightPtr := flag.Int("height", 0, "Block chain height")
	blockIDPtr := flag.String("block_id", "", "Block ID")
	txIDPtr := flag.String("tx_id", "", "Transaction ID")
	startHeightPtr := flag.Int("start_height", 0, "Start block height (for use with \"history\")")
	startIndexPtr := flag.Int("start_index", 0, "Start transaction index (for use with \"history\")")
	endHeightPtr := flag.Int("end_height", 0, "End block height (for use with \"history\")")
	limitPtr := flag.Int("limit", 3, "Limit (for use with \"history\")")
	flag.Parse()

	if len(*dataDirPtr) == 0 {
		log.Printf("You must specify a -datadir\n")
		os.Exit(-1)
	}

	var pubKey ed25519.PublicKey
	if len(*pubKeyPtr) != 0 {
		// decode the key
		pubKeyBytes, err := base64.StdEncoding.DecodeString(*pubKeyPtr)
		if err != nil {
			log.Fatal(err)
		}
		pubKey = ed25519.PublicKey(pubKeyBytes)
	}

	var blockID *BlockID
	if len(*blockIDPtr) != 0 {
		blockIDBytes, err := hex.DecodeString(*blockIDPtr)
		if err != nil {
			log.Fatal(err)
		}
		blockID = new(BlockID)
		copy(blockID[:], blockIDBytes)
	}

	var txID *TransactionID
	if len(*txIDPtr) != 0 {
		txIDBytes, err := hex.DecodeString(*txIDPtr)
		if err != nil {
			log.Fatal(err)
		}
		txID = new(TransactionID)
		copy(txID[:], txIDBytes)
	}

	// instatiate block storage (read-only)
	blockStore, err := NewBlockStorageDisk(
		filepath.Join(*dataDirPtr, "blocks"),
		filepath.Join(*dataDirPtr, "headers.db"),
		true,  // read-only
		false, // compress (if a block is compressed storage will figure it out)
	)
	if err != nil {
		log.Fatal(err)
	}

	// instantiate the ledger (read-only)
	ledger, err := NewLedgerDisk(filepath.Join(*dataDirPtr, "ledger.db"),
		true,  // read-only
		false, // prune (no effect with read-only set)
		blockStore)
	if err != nil {
		log.Fatal(err)
	}

	// get the current height
	_, currentHeight, err := ledger.GetChainTip()
	if err != nil {
		log.Fatal(err)
	}

	switch *cmdPtr {
	case "height":
		log.Printf("Current block chain height is: %d\n", aurora.Bold(currentHeight))

	case "balance":
		if pubKey == nil {
			log.Fatal("-pubkey required for \"balance\" command")
		}
		balance, err := ledger.GetPublicKeyBalance(pubKey)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Current balance: %.8f\n", aurora.Bold(float64(balance)/CRUZBITS_PER_CRUZ))

	case "balance_at":
		if pubKey == nil {
			log.Fatal("-pubkey required for \"balance_at\" command")
		}
		balance, err := ledger.GetPublicKeyBalanceAt(pubKey, int64(*heightPtr))
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Balance at height %d: %.8f\n", *heightPtr, aurora.Bold(float64(balance)/CRUZBITS_PER_CRUZ))

	case "block_at":
		id, err := ledger.GetBlockIDForHeight(int64(*heightPtr))
		if err != nil {
			log.Fatal(err)
		}
		if id == nil {
			log.Fatalf("No block found at height %d\n", *heightPtr)
		}
		block, err := blockStore.GetBlock(*id)
		if err != nil {
			log.Fatal(err)
		}
		if block == nil {
			log.Fatalf("No block with ID %s\n", *id)
		}
		displayBlock(*id, block)

	case "block":
		if blockID == nil {
			log.Fatalf("-block_id required for \"block\" command")
		}
		block, err := blockStore.GetBlock(*blockID)
		if err != nil {
			log.Fatal(err)
		}
		if block == nil {
			log.Fatalf("No block with id %s\n", *blockID)
		}
		displayBlock(*blockID, block)

	case "tx":
		if txID == nil {
			log.Fatalf("-tx_id required for \"tx\" command")
		}
		id, index, err := ledger.GetTransactionIndex(*txID)
		if err != nil {
			log.Fatal(err)
		}
		if id == nil {
			log.Fatalf("Transaction %s not found", *txID)
		}
		tx, header, err := blockStore.GetTransaction(*id, index)
		if err != nil {
			log.Fatal(err)
		}
		if tx == nil {
			log.Fatalf("No transaction found with ID %s\n", *txID)
		}
		displayTransaction(*txID, header, index, tx)

	case "history":
		if pubKey == nil {
			log.Fatal("-pubkey required for \"history\" command")
		}
		bIDs, indices, stopHeight, stopIndex, err := ledger.GetPublicKeyTransactionIndicesRange(
			pubKey, int64(*startHeightPtr), int64(*endHeightPtr), int(*startIndexPtr), int(*limitPtr))
		if err != nil {
			log.Fatal(err)
		}
		displayHistory(bIDs, indices, stopHeight, stopIndex, blockStore)

	case "verify":
		verify(ledger, blockStore, pubKey, currentHeight)
	}

	// close storage
	if err := blockStore.Close(); err != nil {
		log.Println(err)
	}
	if err := ledger.Close(); err != nil {
		log.Println(err)
	}
}

type conciseBlock struct {
	ID           BlockID         `json:"id"`
	Header       BlockHeader     `json:"header"`
	Transactions []TransactionID `json:"transactions"`
}

func displayBlock(id BlockID, block *Block) {
	b := conciseBlock{
		ID:           id,
		Header:       *block.Header,
		Transactions: make([]TransactionID, len(block.Transactions)),
	}

	for i := 0; i < len(block.Transactions); i++ {
		txID, err := block.Transactions[i].ID()
		if err != nil {
			panic(err)
		}
		b.Transactions[i] = txID
	}

	bJson, err := json.MarshalIndent(&b, "", "    ")
	if err != nil {
		panic(err)
	}

	fmt.Println(string(bJson))
}

type txWithContext struct {
	BlockID     BlockID       `json:"block_id"`
	BlockHeader BlockHeader   `json:"block_header"`
	TxIndex     int           `json:"transaction_index_in_block"`
	ID          TransactionID `json:"transaction_id"`
	Transaction *Transaction  `json:"transaction"`
}

func displayTransaction(txID TransactionID, header *BlockHeader, index int, tx *Transaction) {
	blockID, err := header.ID()
	if err != nil {
		panic(err)
	}

	t := txWithContext{
		BlockID:     blockID,
		BlockHeader: *header,
		TxIndex:     index,
		ID:          txID,
		Transaction: tx,
	}

	txJson, err := json.MarshalIndent(&t, "", "    ")
	if err != nil {
		panic(err)
	}

	fmt.Println(string(txJson))
}

type history struct {
	Transactions []txWithContext `json:"transactions"`
}

func displayHistory(bIDs []BlockID, indices []int, stopHeight int64, stopIndex int, blockStore BlockStorage) {
	h := history{Transactions: make([]txWithContext, len(indices))}
	for i := 0; i < len(indices); i++ {
		tx, header, err := blockStore.GetTransaction(bIDs[i], indices[i])
		if err != nil {
			panic(err)
		}
		if tx == nil {
			panic("No transaction found at index")
		}
		txID, err := tx.ID()
		if err != nil {
			panic(err)
		}
		h.Transactions[i] = txWithContext{
			BlockID:     bIDs[i],
			BlockHeader: *header,
			TxIndex:     indices[i],
			ID:          txID,
			Transaction: tx,
		}
	}

	hJson, err := json.MarshalIndent(&h, "", "    ")
	if err != nil {
		panic(err)
	}

	fmt.Println(string(hJson))
}

func verify(ledger Ledger, blockStore BlockStorage, pubKey ed25519.PublicKey, height int64) {
	var err error
	var expect, found int64

	if pubKey == nil {
		// compute expected total balance
		if height-COINBASE_MATURITY >= 0 {
			// sum all mature rewards per schedule
			var i int64
			for i = 0; i <= height-COINBASE_MATURITY; i++ {
				expect += BlockCreationReward(i)
			}

			// account for fees included in immature rewards
			var immatureFees int64
			for i = height - COINBASE_MATURITY + 1; i <= height; i++ {
				if i < 0 {
					continue
				}
				oldID, err := ledger.GetBlockIDForHeight(i)
				if err != nil {
					log.Fatal(err)
				}
				if oldID == nil {
					log.Fatalf("Missing block at height %d\n", i)
				}
				oldTx, _, err := blockStore.GetTransaction(*oldID, 0)
				if err != nil {
					log.Fatal(err)
				}
				if oldTx == nil {
					log.Fatalf("Missing coinbase from block %s\n", *oldID)
				}
				immatureFees += oldTx.Amount - BlockCreationReward(i)
			}
			expect -= immatureFees
		}

		// compute the balance given the sum of all public key balances
		found, err = ledger.Balance()
	} else {
		// get expected balance
		expect, err = ledger.GetPublicKeyBalance(pubKey)
		if err != nil {
			log.Fatal(err)
		}

		// compute the balance based on history
		found, err = ledger.GetPublicKeyBalanceAt(pubKey, height)
		if err != nil {
			log.Fatal(err)
		}
	}

	if err != nil {
		log.Fatal(err)
	}

	if expect != found {
		log.Fatalf("%s: At height %d, we expected %.8f cruz but we found %.8f\n",
			aurora.Bold(aurora.Red("FAILURE")),
			aurora.Bold(height),
			aurora.Bold(float64(expect)/CRUZBITS_PER_CRUZ),
			aurora.Bold(float64(found)/CRUZBITS_PER_CRUZ))
	}

	log.Printf("%s: At height %d, we expected %.8f cruz and we found %.8f\n",
		aurora.Bold(aurora.Green("SUCCESS")),
		aurora.Bold(height),
		aurora.Bold(float64(expect)/CRUZBITS_PER_CRUZ),
		aurora.Bold(float64(found)/CRUZBITS_PER_CRUZ))
}
