// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"encoding/hex"
	"testing"

	"golang.org/x/crypto/ed25519"
)

func TestEncodeBlockHeader(t *testing.T) {
	pubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	// create a coinbase
	tx := NewTransaction(nil, pubKey, INITIAL_COINBASE_REWARD, 0, 0, 0, 0, "hello")

	// create a block
	targetBytes, err := hex.DecodeString(INITIAL_TARGET)
	if err != nil {
		t.Fatal(err)
	}
	var target BlockID
	copy(target[:], targetBytes)
	block, err := NewBlock(BlockID{}, 0, target, BlockID{}, []*Transaction{tx})
	if err != nil {
		t.Fatal(err)
	}

	// encode the header
	encodedHeader, err := encodeBlockHeader(block.Header, 12345)
	if err != nil {
		t.Fatal(err)
	}

	// decode the header
	header, when, err := decodeBlockHeader(encodedHeader)
	if err != nil {
		t.Fatal(err)
	}

	// compare
	if *header != *block.Header {
		t.Fatal("Decoded header doesn't match original")
	}

	if when != 12345 {
		t.Fatal("Decoded timestamp doesn't match original")
	}
}
