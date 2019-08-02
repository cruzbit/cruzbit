// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"github.com/seiflotfy/cuckoofilter"
	"golang.org/x/crypto/ed25519"
)

// Peer is a peer client in the network. They all speak WebSocket protocol to each other.
// Peers could be fully validating and mining nodes or simply wallets.
type Peer struct {
	conn                          *websocket.Conn
	genesisID                     BlockID
	peerStore                     PeerStorage
	blockStore                    BlockStorage
	ledger                        Ledger
	processor                     *Processor
	txQueue                       TransactionQueue
	outbound                      bool
	localDownloadQueue            *BlockQueue // peer-local download queue
	localInflightQueue            *BlockQueue // peer-local inflight queue
	globalInflightQueue           *BlockQueue // global inflight queue
	continuationBlockID           BlockID
	lastPeerAddressesReceivedTime time.Time
	filterLock                    sync.RWMutex
	filter                        *cuckoo.Filter
	addrChan                      chan<- string
	workID                        int32
	workBlock                     *Block
	medianTimestamp               int64
	pubKeys                       []ed25519.PublicKey
	memo                          string
	readLimitLock                 sync.RWMutex
	readLimit                     int64
	closeHandler                  func()
	wg                            sync.WaitGroup
}

// PeerUpgrader upgrades the incoming HTTP connection to a WebSocket if the subprotocol matches.
var PeerUpgrader = websocket.Upgrader{
	Subprotocols: []string{Protocol},
	CheckOrigin:  func(r *http.Request) bool { return true },
}

// NewPeer returns a new instance of a peer.
func NewPeer(conn *websocket.Conn, genesisID BlockID, peerStore PeerStorage,
	blockStore BlockStorage, ledger Ledger, processor *Processor,
	txQueue TransactionQueue, blockQueue *BlockQueue, addrChan chan<- string) *Peer {
	peer := &Peer{
		conn:                conn,
		genesisID:           genesisID,
		peerStore:           peerStore,
		blockStore:          blockStore,
		ledger:              ledger,
		processor:           processor,
		txQueue:             txQueue,
		localDownloadQueue:  NewBlockQueue(),
		localInflightQueue:  NewBlockQueue(),
		globalInflightQueue: blockQueue,
		addrChan:            addrChan,
	}
	peer.updateReadLimit()
	return peer
}

// peerDialer is the websocket.Dialer to use for outbound peer connections
var peerDialer *websocket.Dialer = &websocket.Dialer{
	Proxy:            http.ProxyFromEnvironment,
	HandshakeTimeout: 15 * time.Second,
	Subprotocols:     []string{Protocol}, // set in protocol.go
	TLSClientConfig:  tlsClientConfig,    // set in tls.go
}

// Connect connects outbound to a peer.
func (p *Peer) Connect(ctx context.Context, addr, nonce, myAddr string) (int, error) {
	u := url.URL{Scheme: "wss", Host: addr, Path: "/" + p.genesisID.String()}
	log.Printf("Connecting to %s", u.String())

	if err := p.peerStore.OnConnectAttempt(addr); err != nil {
		return 0, err
	}

	header := http.Header{}
	header.Add("Cruzbit-Peer-Nonce", nonce)
	if len(myAddr) != 0 {
		header.Add("Cruzbit-Peer-Address", myAddr)
	}

	// specify timeout via context. if the parent context is cancelled
	// we'll also abort the connection.
	dialCtx, cancel := context.WithTimeout(ctx, connectWait)
	defer cancel()

	var statusCode int
	conn, resp, err := peerDialer.DialContext(dialCtx, u.String(), header)
	if resp != nil {
		statusCode = resp.StatusCode
	}
	if err != nil {
		if statusCode == http.StatusTooManyRequests {
			// the peer is already connected to us inbound.
			// mark it successful so we try it again in the future.
			p.peerStore.OnConnectSuccess(addr)
			p.peerStore.OnDisconnect(addr)
		} else {
			p.peerStore.OnConnectFailure(addr)
		}
		return statusCode, err
	}

	p.conn = conn
	p.outbound = true
	return statusCode, p.peerStore.OnConnectSuccess(addr)
}

// OnClose specifies a handler to call when the peer connection is closed.
func (p *Peer) OnClose(closeHandler func()) {
	p.closeHandler = closeHandler
}

// Shutdown is called to shutdown the underlying WebSocket synchronously.
func (p *Peer) Shutdown() {
	var addr string
	if p.conn != nil {
		addr = p.conn.RemoteAddr().String()
		p.conn.Close()
	}
	p.wg.Wait()
	if len(addr) != 0 {
		log.Printf("Closed connection with %s\n", addr)
	}
}

const (
	// Time allowed to wait for WebSocket connection
	connectWait = 10 * time.Second

	// Time allowed to write a message to the peer
	writeWait = 30 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 120 * time.Second

	// Send pings to peer with this period. Must be less than pongWait
	pingPeriod = pongWait / 2

	// How often should we refresh this peer's connectivity status with storage
	peerStoreRefreshPeriod = 5 * time.Minute

	// How often should we request peer addresses from a peer
	getPeerAddressesPeriod = 1 * time.Hour

	// Time allowed between processing new blocks before we consider a blockchain sync stalled
	syncWait = 2 * time.Minute

	// Maximum blocks per inv_block message
	maxBlocksPerInv = 500

	// Maximum local inflight queue size
	inflightQueueMax = 8

	// Maximum local download queue size
	downloadQueueMax = maxBlocksPerInv * 10
)

// Run executes the peer's main loop in its own goroutine.
// It manages reading and writing to the peer's WebSocket and facilitating the protocol.
func (p *Peer) Run() {
	p.wg.Add(1)
	go p.run()
}

func (p *Peer) run() {
	defer p.wg.Done()
	if p.closeHandler != nil {
		defer p.closeHandler()
	}

	peerAddr := p.conn.RemoteAddr().String()
	defer func() {
		// remove any inflight blocks this peer is no longer going to download
		blockInflight, ok := p.localInflightQueue.Peek()
		for ok {
			p.localInflightQueue.Remove(blockInflight, "")
			p.globalInflightQueue.Remove(blockInflight, peerAddr)
			blockInflight, ok = p.localInflightQueue.Peek()
		}
	}()

	defer p.conn.Close()

	// written to by the reader loop to send outgoing messages to the writer loop
	outChan := make(chan Message, 1)

	// signals that the reader loop is exiting
	defer close(outChan)

	// send a find common ancestor request and request peer addresses shortly after connecting
	onConnectChan := make(chan bool, 1)
	go func() {
		time.Sleep(5 * time.Second)
		onConnectChan <- true
	}()

	// written to by the reader loop to update the current work block for the peer
	getWorkChan := make(chan GetWorkMessage, 1)
	submitWorkChan := make(chan SubmitWorkMessage, 1)

	// writer goroutine loop
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		// register to hear about tip block changes
		tipChangeChan := make(chan TipChange, 10)
		p.processor.RegisterForTipChange(tipChangeChan)
		defer p.processor.UnregisterForTipChange(tipChangeChan)

		// register to hear about new transactions
		newTxChan := make(chan NewTx, MAX_TRANSACTIONS_TO_INCLUDE_PER_BLOCK)
		p.processor.RegisterForNewTransactions(newTxChan)
		defer p.processor.UnregisterForNewTransactions(newTxChan)

		// send the peer pings
		tickerPing := time.NewTicker(pingPeriod)
		defer tickerPing.Stop()

		// update the peer store with the peer's connectivity
		tickerPeerStoreRefresh := time.NewTicker(peerStoreRefreshPeriod)
		defer tickerPeerStoreRefresh.Stop()

		// request new peer addresses
		tickerGetPeerAddresses := time.NewTicker(getPeerAddressesPeriod)
		defer tickerGetPeerAddresses.Stop()

		// update the peer store on disconnection
		if p.outbound {
			defer p.peerStore.OnDisconnect(peerAddr)
		}

		for {
			select {
			case m, ok := <-outChan:
				if !ok {
					// reader loop is exiting
					return
				}

				// send outgoing message to peer
				p.conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := p.conn.WriteJSON(m); err != nil {
					log.Printf("Write error: %s, to: %s\n", err, p.conn.RemoteAddr())
					p.conn.Close()
				}

			case tip := <-tipChangeChan:
				// update read limit if necessary
				p.updateReadLimit()

				if tip.Connect && tip.More == false {
					// only build off newly connected tip blocks.
					// create and send out new work if necessary
					p.createNewWorkBlock(tip.BlockID, tip.Block.Header)
				}

				if tip.Source == p.conn.RemoteAddr().String() {
					// this is who sent us the block that caused the change
					break
				}

				if tip.Connect {
					// new tip announced, notify the peer
					inv := Message{
						Type: "inv_block",
						Body: InvBlockMessage{
							BlockIDs: []BlockID{tip.BlockID},
						},
					}
					// send it
					p.conn.SetWriteDeadline(time.Now().Add(writeWait))
					if err := p.conn.WriteJSON(inv); err != nil {
						log.Printf("Write error: %s, to: %s\n", err, p.conn.RemoteAddr())
						p.conn.Close()
					}
				}

				// potentially create a filter_block
				fb, err := p.createFilterBlock(tip.BlockID, tip.Block)
				if err != nil {
					log.Printf("Error: %s, to: %s\n", err, p.conn.RemoteAddr())
					continue
				}
				if fb == nil {
					continue
				}

				// send it
				m := Message{
					Type: "filter_block",
					Body: fb,
				}
				if !tip.Connect {
					m.Type = "filter_block_undo"
				}

				log.Printf("Sending %s with %d transaction(s), to: %s\n",
					m.Type, len(fb.Transactions), p.conn.RemoteAddr())
				p.conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := p.conn.WriteJSON(m); err != nil {
					log.Printf("Write error: %s, to: %s\n", err, p.conn.RemoteAddr())
					p.conn.Close()
				}

			case newTx := <-newTxChan:
				// update work if necessary
				p.updateWorkBlock(newTx.TransactionID, newTx.Transaction)

				if newTx.Source == p.conn.RemoteAddr().String() {
					// this is who sent it to us
					break
				}

				interested := func() bool {
					p.filterLock.RLock()
					defer p.filterLock.RUnlock()
					return p.filterLookup(newTx.Transaction)
				}()
				if !interested {
					continue
				}

				// newly verified transaction announced, relay to peer
				pushTx := Message{
					Type: "push_transaction",
					Body: PushTransactionMessage{
						Transaction: newTx.Transaction,
					},
				}
				p.conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := p.conn.WriteJSON(pushTx); err != nil {
					log.Printf("Write error: %s, to: %s\n", err, p.conn.RemoteAddr())
					p.conn.Close()
				}

			case <-onConnectChan:
				// send a new peer a request to find a common ancestor
				if err := p.sendFindCommonAncestor(nil, true, outChan); err != nil {
					log.Printf("Write error: %s, to: %s\n", err, p.conn.RemoteAddr())
					p.conn.Close()
				}

				// send a get_peer_addresses to request peers
				log.Printf("Sending get_peer_addresses to: %s\n", p.conn.RemoteAddr())
				m := Message{Type: "get_peer_addresses"}
				p.conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := p.conn.WriteJSON(m); err != nil {
					log.Printf("Error sending get_peer_addresses: %s, to: %s\n", err, p.conn.RemoteAddr())
					p.conn.Close()
				}

			case gw := <-getWorkChan:
				p.onGetWork(gw)

			case sw := <-submitWorkChan:
				p.onSubmitWork(sw)

			case <-tickerPing.C:
				//log.Printf("Sending ping message to: %s\n", p.conn.RemoteAddr())
				p.conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := p.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					log.Printf("Write error: %s, to: %s\n", err, p.conn.RemoteAddr())
					p.conn.Close()
				}

			case <-tickerPeerStoreRefresh.C:
				if p.outbound == false {
					break
				}
				// periodically refresh our connection time
				if err := p.peerStore.OnConnectSuccess(p.conn.RemoteAddr().String()); err != nil {
					log.Printf("Error from peer store: %s\n", err)
				}

			case <-tickerGetPeerAddresses.C:
				// periodically send a get_peer_addresses
				log.Printf("Sending get_peer_addresses to: %s\n", p.conn.RemoteAddr())
				m := Message{Type: "get_peer_addresses"}
				p.conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := p.conn.WriteJSON(m); err != nil {
					log.Printf("Error sending get_peer_addresses: %s, to: %s\n", err, p.conn.RemoteAddr())
					p.conn.Close()
				}
			}
		}
	}()

	// are we syncing?
	lastNewBlockTime := time.Now()
	ibd, _, err := IsInitialBlockDownload(p.ledger, p.blockStore)
	if err != nil {
		log.Println(err)
		return
	}

	// handle pongs
	p.conn.SetPongHandler(func(string) error {
		if ibd {
			// handle stalled blockchain syncs
			var err error
			ibd, _, err = IsInitialBlockDownload(p.ledger, p.blockStore)
			if err != nil {
				return err
			}
			if ibd && time.Since(lastNewBlockTime) > syncWait {
				return fmt.Errorf("Sync has stalled, disconnecting")
			}
		} else {
			// try processing the queue in case we've been blocked by another client
			// and their attempt has now expired
			if err := p.processDownloadQueue(outChan); err != nil {
				return err
			}
		}
		p.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// set initial read deadline
	p.conn.SetReadDeadline(time.Now().Add(pongWait))

	// reader loop
	for {
		// update read limit
		p.conn.SetReadLimit(p.getReadLimit())

		// new message from peer
		messageType, message, err := p.conn.ReadMessage()
		if err != nil {
			log.Printf("Read error: %s, from: %s\n", err, p.conn.RemoteAddr())
			break
		}

		switch messageType {
		case websocket.TextMessage:
			// sanitize inputs
			if !utf8.Valid(message) {
				log.Printf("Peer sent us non-utf8 clean message, from: %s\n", p.conn.RemoteAddr())
				return
			}

			var body json.RawMessage
			m := Message{Body: &body}
			if err := json.Unmarshal([]byte(message), &m); err != nil {
				log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
				return
			}

			// hangup if the peer is sending oversized messages
			if m.Type != "block" && len(message) > MAX_PROTOCOL_MESSAGE_LENGTH {
				log.Printf("Received too large (%d bytes) of a '%s' message, from: %s",
					len(message), m.Type, p.conn.RemoteAddr())
				return
			}

			switch m.Type {
			case "inv_block":
				var inv InvBlockMessage
				if err := json.Unmarshal(body, &inv); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					return
				}
				for i, id := range inv.BlockIDs {
					if err := p.onInvBlock(id, i, len(inv.BlockIDs), outChan); err != nil {
						log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
						break
					}
				}

			case "get_block":
				var gb GetBlockMessage
				if err := json.Unmarshal(body, &gb); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					return
				}
				if err := p.onGetBlock(gb.BlockID, outChan); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					break
				}

			case "get_block_by_height":
				var gbbh GetBlockByHeightMessage
				if err := json.Unmarshal(body, &gbbh); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					return
				}
				if err := p.onGetBlockByHeight(gbbh.Height, outChan); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					break
				}

			case "block":
				var b BlockMessage
				if err := json.Unmarshal(body, &b); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					return
				}
				if b.Block == nil {
					log.Printf("Error: received nil block, from: %s\n", p.conn.RemoteAddr())
					return
				}
				ok, err := p.onBlock(b.Block, outChan)
				if err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					break
				}
				if ok {
					lastNewBlockTime = time.Now()
				}

			case "find_common_ancestor":
				var fca FindCommonAncestorMessage
				if err := json.Unmarshal(body, &fca); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					return
				}
				num := len(fca.BlockIDs)
				for i, id := range fca.BlockIDs {
					ok, err := p.onFindCommonAncestor(id, i, num, outChan)
					if err != nil {
						log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
						break
					}
					if ok {
						// don't need to process more
						break
					}
				}

			case "get_block_header":
				var gbh GetBlockHeaderMessage
				if err := json.Unmarshal(body, &gbh); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					return
				}
				if err := p.onGetBlockHeader(gbh.BlockID, outChan); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					break
				}

			case "get_block_header_by_height":
				var gbhbh GetBlockHeaderByHeightMessage
				if err := json.Unmarshal(body, &gbhbh); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					return
				}
				if err := p.onGetBlockHeaderByHeight(gbhbh.Height, outChan); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					break
				}

			case "get_balance":
				var gb GetBalanceMessage
				if err := json.Unmarshal(body, &gb); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					return
				}
				if err := p.onGetBalance(gb.PublicKey, outChan); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					break
				}

			case "get_balances":
				var gb GetBalancesMessage
				if err := json.Unmarshal(body, &gb); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					return
				}
				if err := p.onGetBalances(gb.PublicKeys, outChan); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					break
				}

			case "get_public_key_transactions":
				var gpkt GetPublicKeyTransactionsMessage
				if err := json.Unmarshal(body, &gpkt); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					return
				}
				if err := p.onGetPublicKeyTransactions(gpkt.PublicKey,
					gpkt.StartHeight, gpkt.EndHeight, gpkt.StartIndex, gpkt.Limit, outChan); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					break
				}

			case "get_transaction":
				var gt GetTransactionMessage
				if err := json.Unmarshal(body, &gt); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					return
				}
				if err := p.onGetTransaction(gt.TransactionID, outChan); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					break
				}

			case "get_tip_header":
				if err := p.onGetTipHeader(outChan); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					break
				}

			case "push_transaction":
				var pt PushTransactionMessage
				if err := json.Unmarshal(body, &pt); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					return
				}
				if pt.Transaction == nil {
					log.Printf("Error: received nil transaction, from: %s\n", p.conn.RemoteAddr())
					return
				}
				if err := p.onPushTransaction(pt.Transaction, outChan); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					break
				}

			case "push_transaction_result":
				var ptr PushTransactionResultMessage
				if err := json.Unmarshal(body, &ptr); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					return
				}
				if len(ptr.Error) != 0 {
					log.Printf("Error: %s, from: %s\n", ptr.Error, p.conn.RemoteAddr())
				}

			case "filter_load":
				var fl FilterLoadMessage
				if err := json.Unmarshal(body, &fl); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					return
				}
				if err := p.onFilterLoad(fl.Type, fl.Filter, outChan); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					break
				}

			case "filter_add":
				var fa FilterAddMessage
				if err := json.Unmarshal(body, &fa); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					return
				}
				if err := p.onFilterAdd(fa.PublicKeys, outChan); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					break
				}

			case "get_filter_transaction_queue":
				p.onGetFilterTransactionQueue(outChan)

			case "get_peer_addresses":
				if err := p.onGetPeerAddresses(outChan); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					break
				}

			case "peer_addresses":
				var pa PeerAddressesMessage
				if err := json.Unmarshal(body, &pa); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					return
				}
				p.onPeerAddresses(pa.Addresses)

			case "get_transaction_relay_policy":
				outChan <- Message{
					Type: "transaction_relay_policy",
					Body: TransactionRelayPolicyMessage{
						MinFee:    MIN_FEE_CRUZBITS,
						MinAmount: MIN_AMOUNT_CRUZBITS,
					},
				}

			case "get_work":
				var gw GetWorkMessage
				if err := json.Unmarshal(body, &gw); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					return
				}
				log.Printf("Received get_work message, from: %s\n", p.conn.RemoteAddr())
				getWorkChan <- gw

			case "submit_work":
				var sw SubmitWorkMessage
				if err := json.Unmarshal(body, &sw); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					return
				}
				log.Printf("Received submit_work message, from: %s\n", p.conn.RemoteAddr())
				submitWorkChan <- sw

			default:
				log.Printf("Unknown message: %s, from: %s\n", m.Type, p.conn.RemoteAddr())
			}

		case websocket.CloseMessage:
			log.Printf("Received close message from: %s\n", p.conn.RemoteAddr())
			break
		}
	}
}

// Handle a message from a peer indicating block inventory available for download
func (p *Peer) onInvBlock(id BlockID, index, length int, outChan chan<- Message) error {
	log.Printf("Received inv_block: %s, from: %s\n", id, p.conn.RemoteAddr())

	if length > maxBlocksPerInv {
		return fmt.Errorf("%d blocks IDs is more than %d maximum per inv_block",
			length, maxBlocksPerInv)
	}

	// do we have it queued or inflight already?
	if p.localDownloadQueue.Exists(id) || p.localInflightQueue.Exists(id) {
		log.Printf("Block %s is already queued or inflight for download, from: %s\n",
			id, p.conn.RemoteAddr())
		return nil
	}

	// have we processed it?
	branchType, err := p.ledger.GetBranchType(id)
	if err != nil {
		return err
	}
	if branchType != UNKNOWN {
		log.Printf("Already processed block %s", id)
		if length > 1 && index+1 == length {
			// we might be on a deep side chain. this will get us the next 500 blocks
			return p.sendFindCommonAncestor(&id, false, outChan)
		}
		return nil
	}

	if p.localDownloadQueue.Len() >= downloadQueueMax {
		log.Printf("Too many blocks in the download queue %d, max: %d, for: %s",
			p.localDownloadQueue.Len(), downloadQueueMax, p.conn.RemoteAddr())
		// don't return an error just stop adding them to the queue
		return nil
	}

	// add block to this peer's download queue
	p.localDownloadQueue.Add(id, "")

	// process the download queue
	return p.processDownloadQueue(outChan)
}

// Handle a request for a block from a peer
func (p *Peer) onGetBlock(id BlockID, outChan chan<- Message) error {
	log.Printf("Received get_block: %s, from: %s\n", id, p.conn.RemoteAddr())
	return p.getBlock(id, outChan)
}

// Handle a request for a block by height from a peer
func (p *Peer) onGetBlockByHeight(height int64, outChan chan<- Message) error {
	log.Printf("Received get_block_by_height: %d, from: %s\n", height, p.conn.RemoteAddr())
	id, err := p.ledger.GetBlockIDForHeight(height)
	if err != nil {
		// not found
		outChan <- Message{Type: "block"}
		return err
	}
	if id == nil {
		// not found
		outChan <- Message{Type: "block"}
		return fmt.Errorf("No block found at height %d", height)
	}
	return p.getBlock(*id, outChan)
}

func (p *Peer) getBlock(id BlockID, outChan chan<- Message) error {
	// fetch the block
	blockJson, err := p.blockStore.GetBlockBytes(id)
	if err != nil {
		// not found
		outChan <- Message{Type: "block", Body: BlockMessage{BlockID: &id}}
		return err
	}
	if len(blockJson) == 0 {
		// not found
		outChan <- Message{Type: "block", Body: BlockMessage{BlockID: &id}}
		return fmt.Errorf("No block found with ID %s", id)
	}

	// send out the raw bytes
	body := []byte(`{"block_id":"`)
	body = append(body, []byte(id.String())...)
	body = append(body, []byte(`","block":`)...)
	body = append(body, blockJson...)
	body = append(body, []byte(`}`)...)
	outChan <- Message{Type: "block", Body: json.RawMessage(body)}

	// was this the last block in the inv we sent in response to a find common ancestor request?
	if id == p.continuationBlockID {
		log.Printf("Received get_block for continuation block %s, from: %s\n",
			id, p.conn.RemoteAddr())
		p.continuationBlockID = BlockID{}
		// send an inv for our tip block to prompt the peer to
		// send another find common ancestor request to complete its download of the chain.
		tipID, _, err := p.ledger.GetChainTip()
		if err != nil {
			log.Printf("Error: %s\n", err)
			return nil
		}
		if tipID != nil {
			outChan <- Message{Type: "inv_block", Body: InvBlockMessage{BlockIDs: []BlockID{*tipID}}}
		}
	}
	return nil
}

// Handle receiving a block from a peer. Returns true if the block was newly processed and accepted.
func (p *Peer) onBlock(block *Block, outChan chan<- Message) (bool, error) {
	// the message has the ID in it but we can't trust that.
	// it's provided as convenience for trusted peering relationships only
	id, err := block.ID()
	if err != nil {
		return false, err
	}

	log.Printf("Received block: %s, from: %s\n", id, p.conn.RemoteAddr())

	blockInFlight, ok := p.localInflightQueue.Peek()
	if !ok || blockInFlight != id {
		// disconnect misbehaving peer
		p.conn.Close()
		return false, fmt.Errorf("Received unrequested block")
	}

	var accepted bool

	// is it an orphan?
	header, _, err := p.blockStore.GetBlockHeader(block.Header.Previous)
	if err != nil || header == nil {
		p.localInflightQueue.Remove(id, "")
		p.globalInflightQueue.Remove(id, p.conn.RemoteAddr().String())

		if err != nil {
			return false, err
		}

		log.Printf("Block %s is an orphan, sending find_common_ancestor to: %s\n",
			id, p.conn.RemoteAddr())

		// send a find common ancestor request
		if err := p.sendFindCommonAncestor(nil, false, outChan); err != nil {
			return false, err
		}
	} else {
		// process the block
		if err := p.processor.ProcessBlock(id, block, p.conn.RemoteAddr().String()); err != nil {
			// disconnect a peer that sends us a bad block
			p.conn.Close()
			p.localInflightQueue.Remove(id, "")
			p.globalInflightQueue.Remove(id, p.conn.RemoteAddr().String())
			return false, err
		}
		// newly accepted block
		accepted = true

		// remove it from the inflight queues only after we process it
		p.localInflightQueue.Remove(id, "")
		p.globalInflightQueue.Remove(id, p.conn.RemoteAddr().String())
	}

	// see if there are any more blocks to download right now
	if err := p.processDownloadQueue(outChan); err != nil {
		return false, err
	}

	return accepted, nil
}

// Try requesting blocks that are in the download queue
func (p *Peer) processDownloadQueue(outChan chan<- Message) error {
	// fill up as much of the inflight queue as possible
	var queued int
	for p.localInflightQueue.Len() < inflightQueueMax {
		// next block to download
		blockToDownload, ok := p.localDownloadQueue.Peek()
		if !ok {
			// no more blocks in the queue
			break
		}

		// double-check if it's been processed since we last checked
		branchType, err := p.ledger.GetBranchType(blockToDownload)
		if err != nil {
			return err
		}
		if branchType != UNKNOWN {
			// it's been processed. remove it and check the next one
			log.Printf("Block %s has been processed, removing from download queue for: %s\n",
				blockToDownload, p.conn.RemoteAddr().String())
			p.localDownloadQueue.Remove(blockToDownload, "")
			continue
		}

		// add block to the global inflight queue with this peer as the owner
		if p.globalInflightQueue.Add(blockToDownload, p.conn.RemoteAddr().String()) == false {
			// another peer is downloading it right now.
			// wait to see if they succeed before trying to download any others
			log.Printf("Block %s is being downloaded already from another peer\n", blockToDownload)
			break
		}

		// pop it off the local download queue
		p.localDownloadQueue.Remove(blockToDownload, "")

		// mark it inflight locally
		p.localInflightQueue.Add(blockToDownload, "")
		queued++

		// request it
		log.Printf("Sending get_block for %s, to: %s\n", blockToDownload, p.conn.RemoteAddr())
		outChan <- Message{Type: "get_block", Body: GetBlockMessage{BlockID: blockToDownload}}
	}

	if queued > 0 {
		log.Printf("Requested %d block(s) for download, from: %s", queued, p.conn.RemoteAddr())
		log.Printf("Queue size: %d, peer inflight: %d, global inflight: %d, for: %s\n",
			p.localDownloadQueue.Len(), p.localInflightQueue.Len(), p.globalInflightQueue.Len(), p.conn.RemoteAddr())
	}

	return nil
}

// Send a message to look for a common ancestor with a peer
// Might be called from reader or writer context. writeNow means we're in the writer context
func (p *Peer) sendFindCommonAncestor(startID *BlockID, writeNow bool, outChan chan<- Message) error {
	log.Printf("Sending find_common_ancestor to: %s\n", p.conn.RemoteAddr())

	var height int64
	if startID == nil {
		var err error
		startID, height, err = p.ledger.GetChainTip()
		if err != nil {
			return err
		}
	} else {
		header, _, err := p.blockStore.GetBlockHeader(*startID)
		if err != nil {
			return err
		}
		if header == nil {
			return fmt.Errorf("No header for block %s", *startID)
		}
		height = header.Height
	}
	id := startID

	var ids []BlockID
	var step int64 = 1
	for id != nil {
		if *id == p.genesisID {
			break
		}
		ids = append(ids, *id)
		depth := height - step
		if depth <= 0 {
			break
		}
		var err error
		id, err = p.ledger.GetBlockIDForHeight(depth)
		if err != nil {
			log.Printf("Error: %s\n", err)
			return nil
		}
		if len(ids) > 10 {
			step *= 2
		}
		height = depth
	}
	ids = append(ids, p.genesisID)
	m := Message{Type: "find_common_ancestor", Body: FindCommonAncestorMessage{BlockIDs: ids}}

	if writeNow {
		p.conn.SetWriteDeadline(time.Now().Add(writeWait))
		if err := p.conn.WriteJSON(m); err != nil {
			log.Printf("Write error: %s, to: %s\n", err, p.conn.RemoteAddr())
			return err
		}
		return nil
	}
	outChan <- m
	return nil
}

// Handle a find common ancestor message from a peer
func (p *Peer) onFindCommonAncestor(id BlockID, index, length int, outChan chan<- Message) (bool, error) {
	log.Printf("Received find_common_ancestor: %s, index: %d, length: %d, from: %s\n",
		id, index, length, p.conn.RemoteAddr())

	header, _, err := p.blockStore.GetBlockHeader(id)
	if err != nil {
		return false, err
	}
	if header == nil {
		// don't have it
		return false, nil
	}
	branchType, err := p.ledger.GetBranchType(id)
	if err != nil {
		return false, err
	}
	if branchType != MAIN {
		// not on the main branch
		return false, nil
	}

	log.Printf("Common ancestor found: %s, height: %d, with: %s\n",
		id, header.Height, p.conn.RemoteAddr())

	var ids []BlockID
	var height int64 = header.Height + 1
	for len(ids) < maxBlocksPerInv {
		nextID, err := p.ledger.GetBlockIDForHeight(height)
		if err != nil {
			return false, err
		}
		if nextID == nil {
			break
		}
		log.Printf("Queueing inv for block %s, height: %d, to: %s\n",
			nextID, height, p.conn.RemoteAddr())
		ids = append(ids, *nextID)
		height += 1
	}

	if len(ids) > 0 {
		// save the last ID so after the peer requests it we can trigger it to
		// send another find common ancestor request to finish downloading the rest of the chain
		p.continuationBlockID = ids[len(ids)-1]
		log.Printf("Sending inv_block with %d IDs, continuation block: %s, to: %s",
			len(ids), p.continuationBlockID, p.conn.RemoteAddr())
		outChan <- Message{Type: "inv_block", Body: InvBlockMessage{BlockIDs: ids}}
	}
	return true, nil
}

// Handle a request for a block header from a peer
func (p *Peer) onGetBlockHeader(id BlockID, outChan chan<- Message) error {
	log.Printf("Received get_block_header: %s, from: %s\n", id, p.conn.RemoteAddr())
	return p.getBlockHeader(id, outChan)
}

// Handle a request for a block header by ID from a peer
func (p *Peer) onGetBlockHeaderByHeight(height int64, outChan chan<- Message) error {
	log.Printf("Received get_block_header_by_height: %d, from: %s\n", height, p.conn.RemoteAddr())
	id, err := p.ledger.GetBlockIDForHeight(height)
	if err != nil {
		// not found
		outChan <- Message{Type: "block_header"}
		return err
	}
	if id == nil {
		// not found
		outChan <- Message{Type: "block_header"}
		return fmt.Errorf("No block found at height %d", height)
	}
	return p.getBlockHeader(*id, outChan)
}

func (p *Peer) getBlockHeader(id BlockID, outChan chan<- Message) error {
	header, _, err := p.blockStore.GetBlockHeader(id)
	if err != nil {
		// not found
		outChan <- Message{Type: "block_header", Body: BlockHeaderMessage{BlockID: &id}}
		return err
	}
	if header == nil {
		// not found
		outChan <- Message{Type: "block_header", Body: BlockHeaderMessage{BlockID: &id}}
		return fmt.Errorf("Block header for %s not found", id)
	}
	outChan <- Message{Type: "block_header", Body: BlockHeaderMessage{BlockID: &id, BlockHeader: header}}
	return nil
}

// Handle a request for a public key's balance
func (p *Peer) onGetBalance(pubKey ed25519.PublicKey, outChan chan<- Message) error {
	log.Printf("Received get_balance from: %s\n", p.conn.RemoteAddr())

	balances, tipID, tipHeight, err := p.ledger.GetPublicKeyBalances([]ed25519.PublicKey{pubKey})
	if err != nil {
		outChan <- Message{Type: "balance", Body: BalanceMessage{PublicKey: pubKey, Error: err.Error()}}
		return err
	}

	var balance int64
	for _, b := range balances {
		balance = b
	}

	outChan <- Message{
		Type: "balance",
		Body: BalanceMessage{
			BlockID:   tipID,
			Height:    tipHeight,
			PublicKey: pubKey,
			Balance:   balance,
		},
	}
	return nil
}

// Handle a request for a set of public key balances.
func (p *Peer) onGetBalances(pubKeys []ed25519.PublicKey, outChan chan<- Message) error {
	log.Printf("Received get_balances (count: %d) from: %s\n", len(pubKeys), p.conn.RemoteAddr())

	maxPublicKeys := 64
	if len(pubKeys) > maxPublicKeys {
		err := fmt.Errorf("Too many public keys, limit: %d", maxPublicKeys)
		outChan <- Message{Type: "balances", Body: BalancesMessage{Error: err.Error()}}
		return err
	}

	balances, tipID, tipHeight, err := p.ledger.GetPublicKeyBalances(pubKeys)
	if err != nil {
		outChan <- Message{Type: "balances", Body: BalancesMessage{Error: err.Error()}}
		return err
	}

	bm := BalancesMessage{BlockID: tipID, Height: tipHeight}
	bm.Balances = make([]PublicKeyBalance, len(balances))

	i := 0
	for pk, balance := range balances {
		bm.Balances[i] = PublicKeyBalance{PublicKey: ed25519.PublicKey(pk[:]), Balance: balance}
		i++
	}

	outChan <- Message{Type: "balances", Body: bm}
	return nil
}

// Handle a request for a public key's transactions over a given height range
func (p *Peer) onGetPublicKeyTransactions(pubKey ed25519.PublicKey,
	startHeight, endHeight int64, startIndex, limit int, outChan chan<- Message) error {
	log.Printf("Received get_public_key_transactions from: %s\n", p.conn.RemoteAddr())

	if limit < 0 {
		outChan <- Message{Type: "public_key_transactions"}
		return nil
	}

	// enforce our limit
	if limit > 32 || limit == 0 {
		limit = 32
	}

	// get the indices for all transactions for the given public key
	// over the given range of block heights
	bIDs, indices, stopHeight, stopIndex, err := p.ledger.GetPublicKeyTransactionIndicesRange(
		pubKey, startHeight, endHeight, startIndex, limit)
	if err != nil {
		outChan <- Message{Type: "public_key_transactions", Body: PublicKeyTransactionsMessage{Error: err.Error()}}
		return err
	}

	// build filter blocks from the indices
	var fbs []*FilterBlockMessage
	for i, blockID := range bIDs {
		// fetch transaction and header
		tx, blockHeader, err := p.blockStore.GetTransaction(blockID, indices[i])
		if err != nil {
			// odd case. just log it and continue
			log.Printf("Error retrieving transaction history, block: %s, index: %d, error: %s\n",
				blockID, indices[i], err)
			continue
		}
		// figure out where to put it
		var fb *FilterBlockMessage
		if len(fbs) == 0 {
			// new block
			fb = &FilterBlockMessage{BlockID: blockID, Header: blockHeader}
			fbs = append(fbs, fb)
		} else if fbs[len(fbs)-1].BlockID != blockID {
			// new block
			fb = &FilterBlockMessage{BlockID: blockID, Header: blockHeader}
			fbs = append(fbs, fb)
		} else {
			// transaction is from the same block
			fb = fbs[len(fbs)-1]
		}
		fb.Transactions = append(fb.Transactions, tx)
	}

	// send it to the writer
	outChan <- Message{
		Type: "public_key_transactions",
		Body: PublicKeyTransactionsMessage{
			PublicKey:    pubKey,
			StartHeight:  startHeight,
			StopHeight:   stopHeight,
			StopIndex:    stopIndex,
			FilterBlocks: fbs,
		},
	}
	return nil
}

// Handle a request for a transaction
func (p *Peer) onGetTransaction(txID TransactionID, outChan chan<- Message) error {
	log.Printf("Received get_transaction for %s, from: %s\n",
		txID, p.conn.RemoteAddr())

	blockID, index, err := p.ledger.GetTransactionIndex(txID)
	if err != nil {
		// not found
		outChan <- Message{Type: "transaction", Body: TransactionMessage{TransactionID: txID}}
		return err
	}
	if blockID == nil {
		// not found
		outChan <- Message{Type: "transaction", Body: TransactionMessage{TransactionID: txID}}
		return fmt.Errorf("Transaction %s not found", txID)
	}
	tx, header, err := p.blockStore.GetTransaction(*blockID, index)
	if err != nil {
		// odd case but send back what we know at least
		outChan <- Message{Type: "transaction", Body: TransactionMessage{BlockID: blockID, TransactionID: txID}}
		return err
	}
	if tx == nil {
		// another odd case
		outChan <- Message{
			Type: "transaction",
			Body: TransactionMessage{
				BlockID:       blockID,
				Height:        header.Height,
				TransactionID: txID,
			},
		}
		return fmt.Errorf("Transaction at block %s, index %d not found",
			*blockID, index)
	}

	// send it
	outChan <- Message{
		Type: "transaction",
		Body: TransactionMessage{
			BlockID:       blockID,
			Height:        header.Height,
			TransactionID: txID,
			Transaction:   tx,
		},
	}
	return nil
}

// Handle a request for a block header of the tip of the main chain from a peer
func (p *Peer) onGetTipHeader(outChan chan<- Message) error {
	log.Printf("Received get_tip_header, from: %s\n", p.conn.RemoteAddr())
	tipID, tipHeader, tipWhen, err := getChainTipHeader(p.ledger, p.blockStore)
	if err != nil {
		// shouldn't be possible
		outChan <- Message{Type: "tip_header"}
		return err
	}
	outChan <- Message{
		Type: "tip_header",
		Body: TipHeaderMessage{
			BlockID:     tipID,
			BlockHeader: tipHeader,
			TimeSeen:    tipWhen,
		},
	}
	return nil
}

// Handle receiving a transaction from a peer
func (p *Peer) onPushTransaction(tx *Transaction, outChan chan<- Message) error {
	id, err := tx.ID()
	if err != nil {
		outChan <- Message{Type: "push_transaction_result", Body: PushTransactionResultMessage{Error: err.Error()}}
		return err
	}

	log.Printf("Received push_transaction: %s, from: %s\n", id, p.conn.RemoteAddr())

	// process the transaction if this is the first time we've seen it
	var errStr string
	if !p.txQueue.Exists(id) {
		err = p.processor.ProcessTransaction(id, tx, p.conn.RemoteAddr().String())
		if err != nil {
			errStr = err.Error()
		}
	}

	outChan <- Message{Type: "push_transaction_result",
		Body: PushTransactionResultMessage{
			TransactionID: id,
			Error:         errStr,
		},
	}
	return err
}

// Handle a request to set a transaction filter for the connection
func (p *Peer) onFilterLoad(filterType string, filterBytes []byte, outChan chan<- Message) error {
	log.Printf("Received filter_load (size: %d), from: %s\n", len(filterBytes), p.conn.RemoteAddr())

	// check filter type
	if filterType != "cuckoo" {
		err := fmt.Errorf("Unsupported filter type: %s", filterType)
		result := FilterResultMessage{Error: err.Error()}
		outChan <- Message{Type: "filter_result", Body: result}
		return err
	}

	// check limit
	maxSize := 1 << 16
	if len(filterBytes) > maxSize {
		err := fmt.Errorf("Filter too large, max: %d\n", maxSize)
		result := FilterResultMessage{Error: err.Error()}
		outChan <- Message{Type: "filter_result", Body: result}
		return err
	}

	// decode it
	filter, err := cuckoo.Decode(filterBytes)
	if err != nil {
		result := FilterResultMessage{Error: err.Error()}
		outChan <- Message{Type: "filter_result", Body: result}
		return err
	}

	// set the filter
	func() {
		p.filterLock.Lock()
		defer p.filterLock.Unlock()
		p.filter = filter
	}()

	// send the empty result
	outChan <- Message{Type: "filter_result"}
	return nil
}

// Handle a request to add a set of public keys to the filter
func (p *Peer) onFilterAdd(pubKeys []ed25519.PublicKey, outChan chan<- Message) error {
	log.Printf("Received filter_add (public keys: %d), from: %s\n",
		len(pubKeys), p.conn.RemoteAddr())

	// check limit
	maxPublicKeys := 256
	if len(pubKeys) > maxPublicKeys {
		err := fmt.Errorf("Too many public keys, limit: %d", maxPublicKeys)
		result := FilterResultMessage{Error: err.Error()}
		outChan <- Message{Type: "filter_result", Body: result}
		return err
	}

	err := func() error {
		p.filterLock.Lock()
		defer p.filterLock.Unlock()
		// set the filter if it's not set
		if p.filter == nil {
			p.filter = cuckoo.NewFilter(1 << 16)
		}
		// perform the inserts
		for _, pubKey := range pubKeys {
			if !p.filter.Insert(pubKey[:]) {
				return fmt.Errorf("Unable to insert into filter")
			}
		}
		return nil
	}()

	// send the result
	var m Message
	if err != nil {
		m = Message{Type: "filter_result", Body: FilterResultMessage{Error: err.Error()}}
	} else {
		m = Message{Type: "filter_result"}
	}
	outChan <- m
	return nil
}

// Send back a filtered view of the transaction queue
func (p *Peer) onGetFilterTransactionQueue(outChan chan<- Message) {
	log.Printf("Received get_filter_transaction_queue, from: %s\n", p.conn.RemoteAddr())

	ftq := FilterTransactionQueueMessage{}

	p.filterLock.RLock()
	defer p.filterLock.RUnlock()
	if p.filter == nil {
		ftq.Error = "No filter set"
	} else {
		transactions := p.txQueue.Get(0)
		for _, tx := range transactions {
			if p.filterLookup(tx) {
				ftq.Transactions = append(ftq.Transactions, tx)
			}
		}
	}

	outChan <- Message{Type: "filter_transaction_queue", Body: ftq}
}

// Returns true if the transaction is of interest to the peer
func (p *Peer) filterLookup(tx *Transaction) bool {
	if p.filter == nil {
		return true
	}

	if !tx.IsCoinbase() {
		if p.filter.Lookup(tx.From[:]) {
			return true
		}
	}
	return p.filter.Lookup(tx.To[:])
}

// Called from the writer context
func (p *Peer) createFilterBlock(id BlockID, block *Block) (*FilterBlockMessage, error) {
	p.filterLock.RLock()
	defer p.filterLock.RUnlock()

	if p.filter == nil {
		// nothing to do
		return nil, nil
	}

	// create a filter block
	fb := FilterBlockMessage{BlockID: id, Header: block.Header}

	// filter out transactions the peer isn't interested in
	for _, tx := range block.Transactions {
		if p.filterLookup(tx) {
			fb.Transactions = append(fb.Transactions, tx)
		}
	}
	return &fb, nil
}

// Received a request for peer addresses
func (p *Peer) onGetPeerAddresses(outChan chan<- Message) error {
	log.Printf("Received get_peer_addresses message, from: %s\n", p.conn.RemoteAddr())

	// get up to 32 peers that have been connnected to within the last 3 hours
	addresses, err := p.peerStore.GetSince(32, time.Now().Unix()-(60*60*3))
	if err != nil {
		return err
	}

	if len(addresses) != 0 {
		outChan <- Message{Type: "peer_addresses", Body: PeerAddressesMessage{Addresses: addresses}}
	}
	return nil
}

// Received a list of addresses
func (p *Peer) onPeerAddresses(addresses []string) {
	log.Printf("Received peer_addresses message with %d address(es), from: %s\n",
		len(addresses), p.conn.RemoteAddr())

	if time.Since(p.lastPeerAddressesReceivedTime) < (getPeerAddressesPeriod - 2*time.Minute) {
		// don't let a peer flood us with peer addresses
		log.Printf("Ignoring peer addresses, time since last addresses: %v\n",
			time.Now().Sub(p.lastPeerAddressesReceivedTime))
		return
	}
	p.lastPeerAddressesReceivedTime = time.Now()

	limit := 32
	for i, addr := range addresses {
		if i == limit {
			break
		}
		// notify the peer manager
		p.addrChan <- addr
	}
}

// Called from the writer goroutine loop
func (p *Peer) onGetWork(gw GetWorkMessage) {
	var err error
	if p.workBlock != nil {
		err = fmt.Errorf("Peer already has work")
	} else if len(gw.PublicKeys) == 0 {
		err = fmt.Errorf("No public keys specified")
	} else if len(gw.Memo) > MAX_MEMO_LENGTH {
		err = fmt.Errorf("Max memo length (%d) exceeded: %d", MAX_MEMO_LENGTH, len(gw.Memo))
	} else {
		var tipID *BlockID
		var tipHeader *BlockHeader
		tipID, tipHeader, _, err = getChainTipHeader(p.ledger, p.blockStore)
		if err != nil {
			log.Printf("Error getting tip header: %s, for: %s\n", err, p.conn.RemoteAddr())
		} else {
			// create and send out new work
			p.pubKeys = gw.PublicKeys
			p.memo = gw.Memo
			p.createNewWorkBlock(*tipID, tipHeader)
		}
	}

	if err != nil {
		m := Message{Type: "work", Body: WorkMessage{Error: err.Error()}}
		p.conn.SetWriteDeadline(time.Now().Add(writeWait))
		if err := p.conn.WriteJSON(m); err != nil {
			log.Printf("Write error: %s, to: %s\n", err, p.conn.RemoteAddr())
			p.conn.Close()
		}
	}
}

// Create a new work block for a mining peer. Called from the writer goroutine loop.
func (p *Peer) createNewWorkBlock(tipID BlockID, tipHeader *BlockHeader) error {
	if len(p.pubKeys) == 0 {
		// peer doesn't want work
		return nil
	}

	medianTimestamp, err := computeMedianTimestamp(tipHeader, p.blockStore)
	if err != nil {
		log.Printf("Error computing median timestamp: %s, for: %s\n", err, p.conn.RemoteAddr())
	} else {
		// create a new block
		p.medianTimestamp = medianTimestamp
		keyIndex := rand.Intn(len(p.pubKeys))
		p.workID = rand.Int31()
		p.workBlock, err = createNextBlock(tipID, tipHeader, p.txQueue, p.blockStore, p.pubKeys[keyIndex], p.memo)
		if err != nil {
			log.Printf("Error creating next block: %s, for: %s\n", err, p.conn.RemoteAddr())
		}
	}

	m := Message{Type: "work"}
	if err != nil {
		m.Body = WorkMessage{WorkID: p.workID, Error: err.Error()}
	} else {
		m.Body = WorkMessage{WorkID: p.workID, Header: p.workBlock.Header, MinTime: p.medianTimestamp + 1}
	}

	p.conn.SetWriteDeadline(time.Now().Add(writeWait))
	if err := p.conn.WriteJSON(m); err != nil {
		log.Printf("Write error: %s, to: %s\n", err, p.conn.RemoteAddr())
		p.conn.Close()
		return err
	}
	return err
}

// Add the transaction to the current work block for the mining peer. Called from the writer goroutine loop.
func (p *Peer) updateWorkBlock(id TransactionID, tx *Transaction) error {
	if p.workBlock == nil {
		// peer doesn't want work
		return nil
	}

	if MAX_TRANSACTIONS_TO_INCLUDE_PER_BLOCK != 0 &&
		len(p.workBlock.Transactions) >= MAX_TRANSACTIONS_TO_INCLUDE_PER_BLOCK {
		log.Printf("Per-block transaction limit hit (%d)\n", len(p.workBlock.Transactions))
		return nil
	}

	m := Message{Type: "work"}

	// add the transaction to the block (it updates the coinbase fee)
	err := p.workBlock.AddTransaction(id, tx)
	if err != nil {
		log.Printf("Error adding new transaction %s to block: %s\n", id, err)
		// abandon the block
		p.workBlock = nil
		m.Body = WorkMessage{WorkID: p.workID, Error: err.Error()}
	} else {
		// send out the new work
		p.workID = rand.Int31()
		m.Body = WorkMessage{WorkID: p.workID, Header: p.workBlock.Header, MinTime: p.medianTimestamp + 1}
	}

	p.conn.SetWriteDeadline(time.Now().Add(writeWait))
	if err := p.conn.WriteJSON(m); err != nil {
		log.Printf("Write error: %s, to: %s\n", err, p.conn.RemoteAddr())
		p.conn.Close()
		return err
	}

	return err
}

// Handle a submission of mining work. Called from the writer goroutine loop.
func (p *Peer) onSubmitWork(sw SubmitWorkMessage) {
	m := Message{Type: "submit_work_result"}
	id, err := sw.Header.ID()

	if err != nil {
		log.Printf("Error computing block ID: %s, from: %s\n", err, p.conn.RemoteAddr())
	} else if sw.WorkID != p.workID {
		err = fmt.Errorf("Expected work ID %d, found %d", p.workID, sw.WorkID)
		log.Printf("%s, from: %s\n", err.Error(), p.conn.RemoteAddr())
	} else {
		p.workBlock.Header = sw.Header
		err = p.processor.ProcessBlock(id, p.workBlock, p.conn.RemoteAddr().String())
		log.Printf("Error processing work block: %s, for: %s\n", err, p.conn.RemoteAddr())
		p.workBlock = nil
	}

	if err != nil {
		m.Body = SubmitWorkResultMessage{WorkID: sw.WorkID, Error: err.Error()}
	} else {
		m.Body = SubmitWorkResultMessage{WorkID: sw.WorkID}
	}

	p.conn.SetWriteDeadline(time.Now().Add(writeWait))
	if err := p.conn.WriteJSON(m); err != nil {
		log.Printf("Write error: %s, to: %s\n", err, p.conn.RemoteAddr())
		p.conn.Close()
	}
}

// Update the read limit if necessary
func (p *Peer) updateReadLimit() {
	ok, height, err := IsInitialBlockDownload(p.ledger, p.blockStore)
	if err != nil {
		log.Fatal(err)
	}

	p.readLimitLock.Lock()
	defer p.readLimitLock.Unlock()
	if ok {
		// TODO: do something smarter about this
		p.readLimit = 0
		return
	}

	// transactions are <500 bytes so this gives us significant wiggle room
	maxTransactions := computeMaxTransactionsPerBlock(height + 1)
	p.readLimit = int64(maxTransactions) * 1024
}

// Returns the maximum allowed size of a network message
func (p *Peer) getReadLimit() int64 {
	p.readLimitLock.RLock()
	defer p.readLimitLock.RUnlock()
	return p.readLimit
}
