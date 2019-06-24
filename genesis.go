// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

// GenesisBlockJson is the first block in the chain.
// The memo field is the hash of the tip of the bitcoin blockchain at the time of this block's creation.
const GenesisBlockJson = `
{
  "header": {
    "previous": "0000000000000000000000000000000000000000000000000000000000000000",
    "hash_list_root": "7afb89705316b3de79a3882ec3732b6b8796dd4bf2a80240549ae8fd49a517d8",
    "time": 1561173156,
    "target": "00000000ffff0000000000000000000000000000000000000000000000000000",
    "chain_work": "0000000000000000000000000000000000000000000000000000000100010001",
    "nonce": 1695541686981695,
    "height": 0,
    "transaction_count": 1
  },
  "transactions": [
    {
      "time": 1561173126,
      "nonce": 1654479747,
      "to": "ntkSbbG+b0vo49IGd9nnH39eHIxIEqXmIL8aaJZV+jQ=",
      "amount": 5000000000,
      "memo": "0000000000000000000de6d595bddae743ac032b1458a47ccaef7b0f6f1e3210",
      "series": 1
    }
  ]
}`
