// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"encoding/hex"
	"fmt"
	"golang.org/x/crypto/ed25519"
	"strconv"
	"strings"
	"testing"
)

// create a deterministic test block
func makeTestBlock(n int) (*Block, error) {
	txs := make([]*Transaction, n)

	// create txs
	for i := 0; i < n; i++ {
		// create a sender
		seed := strings.Repeat(strconv.Itoa(i%10), ed25519.SeedSize)
		privKey := ed25519.NewKeyFromSeed([]byte(seed))
		pubKey := privKey.Public().(ed25519.PublicKey)

		// create a recipient
		seed2 := strings.Repeat(strconv.Itoa((i+1)%10), ed25519.SeedSize)
		privKey2 := ed25519.NewKeyFromSeed([]byte(seed2))
		pubKey2 := privKey2.Public().(ed25519.PublicKey)

		matures := MAX_NUMBER
		expires := MAX_NUMBER
		height := MAX_NUMBER
		amount := int64(MAX_MONEY)
		fee := int64(MAX_MONEY)

		tx := NewTransaction(pubKey, pubKey2, amount, fee, matures, height, expires, "こんにちは")
		if len(tx.Memo) != 15 {
			// make sure len() gives us bytes not rune count
			return nil, fmt.Errorf("Expected memo length to be 15 but received %d", len(tx.Memo))
		}
		tx.Nonce = int32(123456789 + i)

		// sign the transaction
		if err := tx.Sign(privKey); err != nil {
			return nil, err
		}
		txs[i] = tx
	}

	// create the block
	targetBytes, err := hex.DecodeString(INITIAL_TARGET)
	if err != nil {
		return nil, err
	}
	var target BlockID
	copy(target[:], targetBytes)
	block, err := NewBlock(BlockID{}, 0, target, BlockID{}, txs)
	if err != nil {
		return nil, err
	}
	return block, nil
}

func TestBlockHeaderHasher(t *testing.T) {
	block, err := makeTestBlock(10)
	if err != nil {
		t.Fatal(err)
	}

	if !compareIDs(block) {
		t.Fatal("ID mismatch 1")
	}

	block.Header.Time = 1234

	if !compareIDs(block) {
		t.Fatal("ID mismatch 2")
	}

	block.Header.Nonce = 1234

	if !compareIDs(block) {
		t.Fatal("ID mismatch 3")
	}

	block.Header.Nonce = 1235

	if !compareIDs(block) {
		t.Fatal("ID mismatch 4")
	}

	block.Header.Nonce = 1236
	block.Header.Time = 1234

	if !compareIDs(block) {
		t.Fatal("ID mismatch 5")
	}

	block.Header.Time = 123498
	block.Header.Nonce = 12370910

	txID, _ := block.Transactions[0].ID()
	if err := block.AddTransaction(txID, block.Transactions[0]); err != nil {
		t.Fatal(err)
	}

	if !compareIDs(block) {
		t.Fatal("ID mismatch 6")
	}

	block.Header.Time = 987654321

	if !compareIDs(block) {
		t.Fatal("ID mismatch 7")
	}
}

func compareIDs(block *Block) bool {
	// compute header ID
	id, _ := block.ID()

	// use delta method
	idInt, _ := block.Header.IDFast(0)
	id2 := new(BlockID).SetBigInt(idInt)
	return id == *id2
}
