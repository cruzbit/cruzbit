// +build !opencl
// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

const OPENCL_ENABLED = false

func OpenCLInit() int {
	return 0
}

func OpenCLMinerUpdate(minerNum int, headerBytes []byte, headerBytesLen, startNonceOffset, endNonceOffset int, target BlockID) int64 {
	return 0
}

func OpenCLMinerMine(minerNum int, startNonce int64) int64 {
	return 0
}
