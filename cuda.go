// +build cuda
// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

//#cgo LDFLAGS: -L./cuda/build -lcuda
//
// #include <stdint.h>
//
// extern int cuda_init();
// extern int miner_update(int miner_num, const void *first, size_t first_len, const void *last,
//     size_t last_len, const void *target);
// extern int64_t miner_mine(int miner_num, int64_t start_nonce);
//
import "C"

import (
	"unsafe"
)

const CUDA_ENABLED = true

// CudaInit is called on startup.
func CudaInit() int {
	return int(C.cuda_init())
}

// CudaMinerUpdate is called by a miner goroutine when the underlying header changes.
func CudaMinerUpdate(minerNum int, headerBytes []byte, headerBytesLen, startNonceOffset, endNonceOffset int, target BlockID) int64 {
	return int64(C.miner_update(C.int(minerNum), unsafe.Pointer(&headerBytes[0]), C.size_t(startNonceOffset),
		unsafe.Pointer(&headerBytes[endNonceOffset]), C.size_t(headerBytesLen-endNonceOffset),
		unsafe.Pointer(&target[0])))
}

// CudaMine is called on every solution attempt by a miner goroutine.
// It will perform N hashing attempts where N is the maximum number of threads your device is capable of executing.
// Returns a solving nonce; otherwise 0x7FFFFFFFFFFFFFFF.
func CudaMinerMine(minerNum int, startNonce int64) int64 {
	return int64(C.miner_mine(C.int(minerNum), C.int64_t(startNonce)))
}
