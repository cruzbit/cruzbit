// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"encoding/hex"
	"hash"
	"log"
	"math/big"
	"strconv"

	"golang.org/x/crypto/sha3"
)

// BlockHeaderHasher is used to more efficiently hash JSON serialized block headers while mining.
type BlockHeaderHasher struct {
	// these can change per attempt
	previousHashListRoot     TransactionID
	previousTime             int64
	previousNonce            int64
	previousTransactionCount int32

	// used for tracking offsets of mutable fields in the buffer
	hashListRootOffset     int
	timeOffset             int
	nonceOffset            int
	transactionCountOffset int

	// used for calculating a running offset
	timeLen    int
	nonceLen   int
	txCountLen int

	// used for hashing
	initialized      bool
	bufLen           int
	buffer           []byte
	hasher           HashWithRead
	resultBuf        [32]byte
	result           *big.Int
	hashesPerAttempt int64
}

// HashWithRead extends hash.Hash to provide a Read interface.
type HashWithRead interface {
	hash.Hash

	// the sha3 state objects aren't exported from stdlib but some of their methods like Read are.
	// we can get the sum without the clone done by Sum which saves us a malloc in the fast path
	Read(out []byte) (n int, err error)
}

// Static fields
var hdrPrevious []byte = []byte(`{"previous":"`)
var hdrHashListRoot []byte = []byte(`","hash_list_root":"`)
var hdrTime []byte = []byte(`","time":`)
var hdrTarget []byte = []byte(`,"target":"`)
var hdrChainWork []byte = []byte(`","chain_work":"`)
var hdrNonce []byte = []byte(`","nonce":`)
var hdrHeight []byte = []byte(`,"height":`)
var hdrTransactionCount []byte = []byte(`,"transaction_count":`)
var hdrEnd []byte = []byte("}")

// NewBlockHeaderHasher returns a newly initialized BlockHeaderHasher
func NewBlockHeaderHasher() *BlockHeaderHasher {
	// calculate the maximum buffer length needed
	bufLen := len(hdrPrevious) + len(hdrHashListRoot) + len(hdrTime) + len(hdrTarget)
	bufLen += len(hdrChainWork) + len(hdrNonce) + len(hdrHeight) + len(hdrTransactionCount)
	bufLen += len(hdrEnd) + 4*64 + 3*19 + 10

	// initialize the hasher
	return &BlockHeaderHasher{
		buffer:           make([]byte, bufLen),
		hasher:           sha3.New256().(HashWithRead),
		result:           new(big.Int),
		hashesPerAttempt: 1,
	}
}

// Initialize the buffer to be hashed
func (h *BlockHeaderHasher) initBuffer(header *BlockHeader) {
	// lots of mixing append on slices with writes to array offsets.
	// pretty annoying that hex.Encode and strconv.AppendInt don't have a consistent interface

	// previous
	copy(h.buffer[:], hdrPrevious)
	bufLen := len(hdrPrevious)
	written := hex.Encode(h.buffer[bufLen:], header.Previous[:])
	bufLen += written

	// hash_list_root
	h.previousHashListRoot = header.HashListRoot
	copy(h.buffer[bufLen:], hdrHashListRoot)
	bufLen += len(hdrHashListRoot)
	h.hashListRootOffset = bufLen
	written = hex.Encode(h.buffer[bufLen:], header.HashListRoot[:])
	bufLen += written

	// time
	h.previousTime = header.Time
	copy(h.buffer[bufLen:], hdrTime)
	bufLen += len(hdrTime)
	h.timeOffset = bufLen
	buffer := strconv.AppendInt(h.buffer[:bufLen], header.Time, 10)
	h.timeLen = len(buffer[bufLen:])
	bufLen += h.timeLen

	// target
	buffer = append(buffer, hdrTarget...)
	bufLen += len(hdrTarget)
	written = hex.Encode(h.buffer[bufLen:], header.Target[:])
	bufLen += written

	// chain_work
	copy(h.buffer[bufLen:], hdrChainWork)
	bufLen += len(hdrChainWork)
	written = hex.Encode(h.buffer[bufLen:], header.ChainWork[:])
	bufLen += written

	// nonce
	h.previousNonce = header.Nonce
	copy(h.buffer[bufLen:], hdrNonce)
	bufLen += len(hdrNonce)
	h.nonceOffset = bufLen
	buffer = strconv.AppendInt(h.buffer[:bufLen], header.Nonce, 10)
	h.nonceLen = len(buffer[bufLen:])
	bufLen += h.nonceLen

	// height
	buffer = append(buffer, hdrHeight...)
	bufLen += len(hdrHeight)
	buffer = strconv.AppendInt(buffer, header.Height, 10)
	bufLen += len(buffer[bufLen:])

	// transaction_count
	h.previousTransactionCount = header.TransactionCount
	buffer = append(buffer, hdrTransactionCount...)
	bufLen += len(hdrTransactionCount)
	h.transactionCountOffset = bufLen
	buffer = strconv.AppendInt(buffer, int64(header.TransactionCount), 10)
	h.txCountLen = len(buffer[bufLen:])
	bufLen += h.txCountLen

	buffer = append(buffer, hdrEnd[:]...)
	h.bufLen = len(buffer[bufLen:]) + bufLen

	h.initialized = true
}

// Update is called everytime the header is updated and the caller wants its new hash value/ID.
func (h *BlockHeaderHasher) Update(minerNum int, header *BlockHeader) (*big.Int, int64) {
	if !h.initialized {
		h.initBuffer(header)
		if CUDA_ENABLED {
			lastOffset := h.nonceOffset + h.nonceLen
			h.hashesPerAttempt = CudaMinerUpdate(minerNum, h.buffer, h.bufLen,
				h.nonceOffset, lastOffset, header.Target)
		}
	} else {
		var bufferChanged bool

		// hash_list_root
		if h.previousHashListRoot != header.HashListRoot {
			bufferChanged = true
			// write out the new value
			h.previousHashListRoot = header.HashListRoot
			hex.Encode(h.buffer[h.hashListRootOffset:], header.HashListRoot[:])
		}

		var offset int

		// time
		if h.previousTime != header.Time {
			bufferChanged = true
			h.previousTime = header.Time

			// write out the new value
			bufLen := h.timeOffset
			buffer := strconv.AppendInt(h.buffer[:bufLen], header.Time, 10)
			timeLen := len(buffer[bufLen:])
			bufLen += timeLen

			// did time shrink or grow in length?
			offset = timeLen - h.timeLen
			h.timeLen = timeLen

			if offset != 0 {
				// shift everything below up or down

				// target
				copy(h.buffer[bufLen:], hdrTarget)
				bufLen += len(hdrTarget)
				written := hex.Encode(h.buffer[bufLen:], header.Target[:])
				bufLen += written

				// chain_work
				copy(h.buffer[bufLen:], hdrChainWork)
				bufLen += len(hdrChainWork)
				written = hex.Encode(h.buffer[bufLen:], header.ChainWork[:])
				bufLen += written

				// start of nonce
				copy(h.buffer[bufLen:], hdrNonce)
			}
		}

		// nonce
		if offset != 0 || (!CUDA_ENABLED && h.previousNonce != header.Nonce) {
			bufferChanged = true
			h.previousNonce = header.Nonce

			// write out the new value (or old value at a new location)
			h.nonceOffset += offset
			bufLen := h.nonceOffset
			buffer := strconv.AppendInt(h.buffer[:bufLen], header.Nonce, 10)
			nonceLen := len(buffer[bufLen:])

			// did nonce shrink or grow in length?
			offset += nonceLen - h.nonceLen
			h.nonceLen = nonceLen

			if offset != 0 {
				// shift everything below up or down

				// height
				buffer = append(buffer, hdrHeight...)
				buffer = strconv.AppendInt(buffer, header.Height, 10)

				// start of transaction_count
				buffer = append(buffer, hdrTransactionCount...)
			}
		}

		// transaction_count
		if offset != 0 || h.previousTransactionCount != header.TransactionCount {
			bufferChanged = true
			h.previousTransactionCount = header.TransactionCount

			// write out the new value (or old value at a new location)
			h.transactionCountOffset += offset
			bufLen := h.transactionCountOffset
			buffer := strconv.AppendInt(h.buffer[:bufLen], int64(header.TransactionCount), 10)
			txCountLen := len(buffer[bufLen:])

			// did count shrink or grow in length?
			offset += txCountLen - h.txCountLen
			h.txCountLen = txCountLen

			if offset != 0 {
				// shift the footer up or down
				buffer = append(buffer, hdrEnd...)
			}
		}

		// it's possible (likely) we did a bunch of encoding with no net impact to the buffer length
		h.bufLen += offset

		if CUDA_ENABLED && bufferChanged {
			// something besides the nonce changed since last time. update the buffers in CUDA.
			lastOffset := h.nonceOffset + h.nonceLen
			h.hashesPerAttempt = CudaMinerUpdate(minerNum, h.buffer, h.bufLen,
				h.nonceOffset, lastOffset, header.Target)
		}
	}

	if CUDA_ENABLED {
		var nonce int64 = CudaMinerMine(minerNum, header.Nonce)
		if nonce == 0x7FFFFFFFFFFFFFFF {
			h.result.SetBytes(
				// indirectly let miner.go know we failed
				[]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
					0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
					0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
					0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			)
			return h.result, h.hashesPerAttempt
		} else {
			log.Printf("CUDA miner %d found a possible solution: %d, double-checking it...\n", minerNum, nonce)
			// rebuild the buffer with the new nonce since we don't update it
			// per attempt when using CUDA.
			header.Nonce = nonce
			h.initBuffer(header)
		}
	}

	// hash it
	h.hasher.Reset()
	h.hasher.Write(h.buffer[:h.bufLen])
	h.hasher.Read(h.resultBuf[:])
	h.result.SetBytes(h.resultBuf[:])
	return h.result, h.hashesPerAttempt
}
