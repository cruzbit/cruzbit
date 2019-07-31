// +build opencl
// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

//#cgo LDFLAGS: -lcruzbit_ocl
//
// #include <stdint.h>
//
// extern int ocl_init();
// extern int miner_update(int miner_num, const void *first, size_t first_len, const void *last,
//     size_t last_len, const void *target);
// extern int64_t miner_mine(int miner_num, int64_t start_nonce);
//
import "C"

import (
	"unsafe"
)

const OPENCL_ENABLED = true

// OpenCLInit is called on startup.
func OpenCLInit() int {
	return int(C.ocl_init())
}

// OpenCLMinerUpdate is called by a miner goroutine when the underlying header changes.
func OpenCLMinerUpdate(minerNum int, headerBytes []byte, headerBytesLen, startNonceOffset, endNonceOffset int, target BlockID) int64 {
	return int64(C.miner_update(C.int(minerNum), unsafe.Pointer(&headerBytes[0]), C.size_t(startNonceOffset),
		unsafe.Pointer(&headerBytes[endNonceOffset]), C.size_t(headerBytesLen-endNonceOffset),
		unsafe.Pointer(&target[0])))
}

// OpenCLMinerMine is called on every solution attempt by a miner goroutine.
// It will perform N hashing attempts where N is the maximum number of threads your device is capable of executing.
// Returns a solving nonce; otherwise 0x7FFFFFFFFFFFFFFF.
func OpenCLMinerMine(minerNum int, startNonce int64) int64 {
	return int64(C.miner_mine(C.int(minerNum), C.int64_t(startNonce)))
}
