// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

// the below values affect ledger consensus and come directly from bitcoin.
// we could have played with these but we're introducing significant enough changes
// already IMO, so let's keep the scope of this experiment as small as we can

const CRUZBITS_PER_CRUZ = 100000000

const INITIAL_COINBASE_REWARD = 50 * CRUZBITS_PER_CRUZ

const COINBASE_MATURITY = 100 // blocks

const INITIAL_TARGET = "00000000ffff0000000000000000000000000000000000000000000000000000"

const MAX_FUTURE_SECONDS = 2 * 60 * 60 // 2 hours

const MAX_MONEY = 21000000 * CRUZBITS_PER_CRUZ

const RETARGET_INTERVAL = 2016 // 2 weeks in blocks

const RETARGET_TIME = 1209600 // 2 weeks in seconds

const TARGET_SPACING = 600 // every 10 minutes

const NUM_BLOCKS_FOR_MEDIAN_TMESTAMP = 11

const BLOCKS_UNTIL_REWARD_HALVING = 210000 // 4 years in blocks

// the below value affects ledger consensus and comes from bitcoin cash

const RETARGET_SMA_WINDOW = 144 // 1 day in blocks

// the below values affect ledger consensus and are new as of our ledger

const INITIAL_MAX_TRANSACTIONS_PER_BLOCK = 10000 // 16.666... tx/sec, ~4 MBish in JSON

const BLOCKS_UNTIL_TRANSACTIONS_PER_BLOCK_DOUBLING = 105000 // 2 years in blocks

const MAX_TRANSACTIONS_PER_BLOCK = 1<<31 - 1

const MAX_TRANSACTIONS_PER_BLOCK_EXCEEDED_AT_HEIGHT = 1852032 // pre-calculated

const BLOCKS_UNTIL_NEW_SERIES = 1008 // 1 week in blocks

const MAX_MEMO_LENGTH = 100 // bytes (ascii/utf8 only)

// given our JSON protocol we should respect Javascript's Number.MAX_SAFE_INTEGER value
const MAX_NUMBER int64 = 1<<53 - 1

// height at which we switch from bitcoin's difficulty adjustment algorithm to bitcoin cash's algorithm
const BITCOIN_CASH_RETARGET_ALGORITHM_HEIGHT = 28861

// the below values only affect peering behavior and do not affect ledger consensus

const DEFAULT_CRUZBIT_PORT = 8831

const MAX_OUTBOUND_PEER_CONNECTIONS = 8

const MAX_INBOUND_PEER_CONNECTIONS = 128

const MAX_INBOUND_PEER_CONNECTIONS_FROM_SAME_HOST = 4

const MAX_TIP_AGE = 24 * 60 * 60

const MAX_PROTOCOL_MESSAGE_LENGTH = 2 * 1024 * 1024 // doesn't apply to blocks

// the below values are mining policy and also do not affect ledger consensus

// if you change this it needs to be less than the maximum at the current height
const MAX_TRANSACTIONS_TO_INCLUDE_PER_BLOCK = INITIAL_MAX_TRANSACTIONS_PER_BLOCK

const MAX_TRANSACTION_QUEUE_LENGTH = MAX_TRANSACTIONS_TO_INCLUDE_PER_BLOCK * 10

const MIN_FEE_CRUZBITS = 1000000 // 0.01 cruz

const MIN_AMOUNT_CRUZBITS = 1000000 // 0.01 cruz
