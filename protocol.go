// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import "golang.org/x/crypto/ed25519"

// Protocol is the name of this version of the cruzbit peer protocol.
const Protocol = "cruzbit.1"

// Message is a message frame for all messages in the cruzbit.1 protocol.
type Message struct {
	Type string      `json:"type"`
	Body interface{} `json:"body,omitempty"`
}

// InvBlockMessage is used to communicate blocks available for download.
// Type: "inv_block".
type InvBlockMessage struct {
	BlockIDs []BlockID `json:"block_ids"`
}

// GetBlockMessage is used to request a block for download.
// Type: "get_block".
type GetBlockMessage struct {
	BlockID BlockID `json:"block_id"`
}

// GetBlockByHeightMessage is used to request a block for download.
// Type: "get_block_by_height".
type GetBlockByHeightMessage struct {
	Height int64 `json:"height"`
}

// BlockMessage is used to send a peer a complete block.
// Type: "block".
type BlockMessage struct {
	BlockID *BlockID `json:"block_id,omitempty"`
	Block   *Block   `json:"block,omitempty"`
}

// GetBlockHeaderMessage is used to request a block header.
// Type: "get_block_header".
type GetBlockHeaderMessage struct {
	BlockID BlockID `json:"block_id"`
}

// GetBlockHeaderByHeghtMessage is used to request a block header.
// Type: "get_block_header_by_height".
type GetBlockHeaderByHeightMessage struct {
	Height int64 `json:"height"`
}

// BlockHeaderMessage is used to send a peer a block's heder.
// Type: "block_header".
type BlockHeaderMessage struct {
	BlockID     *BlockID     `json:"block_id,omitempty"`
	BlockHeader *BlockHeader `json:"header,omitempty"`
}

// FindCommonAncestorMessage is used to find a common ancestor with a peer.
// Type: "find_common_ancestor".
type FindCommonAncestorMessage struct {
	BlockIDs []BlockID `json:"block_ids"`
}

// GetBalanceMessage requests a public key's balance.
// Type: "get_balance".
type GetBalanceMessage struct {
	PublicKey ed25519.PublicKey `json:"public_key"`
}

// BalanceMessage is used to send a public key's balance to a peer.
// Type: "balance".
type BalanceMessage struct {
	BlockID   *BlockID          `json:"block_id,omitempty"`
	Height    int64             `json:"height,omitempty"`
	PublicKey ed25519.PublicKey `json:"public_key"`
	Balance   int64             `json:"balance"`
	Error     string            `json:"error,omitempty"`
}

// GetBalancesMessage requests a set of public key balances.
// Type: "get_balances".
type GetBalancesMessage struct {
	PublicKeys []ed25519.PublicKey `json:"public_keys"`
}

// BalancesMessage is used to send a public key balances to a peer.
// Type: "balances".
type BalancesMessage struct {
	BlockID  *BlockID           `json:"block_id,omitempty"`
	Height   int64              `json:"height,omitempty"`
	Balances []PublicKeyBalance `json:"balances,omitempty"`
	Error    string             `json:"error,omitempty"`
}

// PublicKeyBalance is an entry in the BalancesMessage's Balances field.
type PublicKeyBalance struct {
	PublicKey ed25519.PublicKey `json:"public_key"`
	Balance   int64             `json:"balance"`
}

// GetTransactioMessage is used to request a confirmed transaction.
// Type: "get_transaction".
type GetTransactionMessage struct {
	TransactionID TransactionID `json:"transaction_id"`
}

// TransactionMessage us used to send a peer a confirmed transaction.
// Type: "transaction"
type TransactionMessage struct {
	BlockID       *BlockID      `json:"block_id,omitempty"`
	Height        int64         `json:"height,omitempty"`
	TransactionID TransactionID `json:"transaction_id"`
	Transaction   *Transaction  `json:"transaction,omitempty"`
}

// TipHeaderMessage is used to send a peer the header for the tip block in the block chain.
// Type: "tip_header". It is sent in response to the empty "get_tip_header" message type.
type TipHeaderMessage struct {
	BlockID     *BlockID     `json:"block_id,omitempty"`
	BlockHeader *BlockHeader `json:"header,omitempty"`
	TimeSeen    int64        `json:"time_seen,omitempty"`
}

// PushTransactionMessage is used to push a newly processed unconfirmed transaction to peers.
// Type: "push_transaction".
type PushTransactionMessage struct {
	Transaction *Transaction `json:"transaction"`
}

// PushTransactionResultMessage is sent in response to a PushTransactionMessage.
// Type: "push_transaction_result".
type PushTransactionResultMessage struct {
	TransactionID TransactionID `json:"transaction_id"`
	Error         string        `json:"error,omitempty"`
}

// FilterLoadMessage is used to request that we load a filter which is used to
// filter transactions returned to the peer based on interest.
// Type: "filter_load"
type FilterLoadMessage struct {
	Type   string `json:"type"`
	Filter []byte `json:"filter"`
}

// FilterAddMessage is used to request the addition of the given public keys to the current filter.
// The filter is created if it's not set.
// Type: "filter_add".
type FilterAddMessage struct {
	PublicKeys []ed25519.PublicKey `json:"public_keys"`
}

// FilterResultMessage indicates whether or not the filter request was successful.
// Type: "filter_result".
type FilterResultMessage struct {
	Error string `json:"error,omitempty"`
}

// FilterBlockMessage represents a pared down block containing only transactions relevant to the peer given their filter.
// Type: "filter_block".
type FilterBlockMessage struct {
	BlockID      BlockID        `json:"block_id"`
	Header       *BlockHeader   `json:"header"`
	Transactions []*Transaction `json:"transactions"`
}

// FilterTransactionQueueMessage returns a pared down view of the unconfirmed transaction queue containing only
// transactions relevant to the peer given their filter.
// Type: "filter_transaction_queue".
type FilterTransactionQueueMessage struct {
	Transactions []*Transaction `json:"transactions"`
	Error        string         `json:"error,omitempty"`
}

// GetPublicKeyTransactionsMessage requests transactions associated with a given public key over a given
// height range of the block chain.
// Type: "get_public_key_transactions".
type GetPublicKeyTransactionsMessage struct {
	PublicKey   ed25519.PublicKey `json:"public_key"`
	StartHeight int64             `json:"start_height"`
	StartIndex  int               `json:"start_index"`
	EndHeight   int64             `json:"end_height"`
	Limit       int               `json:"limit"`
}

// PublicKeyTransactionsMessage is used to return a list of block headers and the transactions relevant to
// the public key over a given height range of the block chain.
// Type: "public_key_transactions".
type PublicKeyTransactionsMessage struct {
	PublicKey    ed25519.PublicKey     `json:"public_key"`
	StartHeight  int64                 `json:"start_height"`
	StopHeight   int64                 `json:"stop_height"`
	StopIndex    int                   `json:"stop_index"`
	FilterBlocks []*FilterBlockMessage `json:"filter_blocks"`
	Error        string                `json:"error,omitempty"`
}

// PeerAddressesMessage is used to communicate a list of potential peer addresses known by a peer.
// Type: "peer_addresses". Sent in response to the empty "get_peer_addresses" message type.
type PeerAddressesMessage struct {
	Addresses []string `json:"addresses"`
}

// TransactionRelayPolicyMessage is used to communicate this node's current settings for min fee and min amount.
// Type: "transaction_relay_policy". Sent in response to the empty "get_transaction_relay_policy" message type.
type TransactionRelayPolicyMessage struct {
	MinFee    int64 `json:"min_fee"`
	MinAmount int64 `json:"min_amount"`
}

// GetWorkMessage is used by a mining peer to request mining work.
// Type: "get_work"
type GetWorkMessage struct {
	PublicKeys []ed25519.PublicKey `json:"public_keys"`
	Memo       string              `json:"memo,omitempty"`
}

// WorkMessage is used by a client to send work to perform to a mining peer.
// The timestamp and nonce in the header can be manipulated by the mining peer.
// It is the mining peer's responsibility to ensure the timestamp is not set below
// the minimum timestamp and that the nonce does not exceed MAX_NUMBER (2^53-1).
// Type: "work"
type WorkMessage struct {
	Header  *BlockHeader `json:"header"`
	MinTime int64        `json:"min_time"`
	Error   string       `json:"error,omitempty"`
}

// SubmitWorkMessage is used by a mining peer to submit a potential solution to the client.
// Type: "submit_work"
type SubmitWorkMessage struct {
	BlockID BlockID      `json:"block_id"`
	Header  *BlockHeader `json:"header"`
}

// SubmitWorkResultMessage is used to inform a mining peer of the result of its work.
// Type: "submit_work_result"
type SubmitWorkResultMessage struct {
	Error string `json:"error,omitempty"`
}
