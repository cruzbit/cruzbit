// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/seiflotfy/cuckoofilter"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/nacl/secretbox"
)

// Wallet manages keys and transactions on behalf of a user.
type Wallet struct {
	db                  *leveldb.DB
	passphrase          string
	conn                *websocket.Conn
	outChan             chan Message      // outgoing messages for synchronous requests
	resultChan          chan walletResult // incoming results for synchronous requests
	transactionCallback func(*Transaction)
	filterBlockCallback func(*FilterBlockMessage)
	filter              *cuckoo.Filter
	wg                  sync.WaitGroup
}

// NewWallet returns a new Wallet instance.
func NewWallet(walletDbPath string) (*Wallet, error) {
	db, err := leveldb.OpenFile(walletDbPath, nil)
	if err != nil {
		return nil, err
	}
	w := &Wallet{db: db}
	if err := w.initializeFilter(); err != nil {
		w.db.Close()
		return nil, err
	}
	return w, nil
}

func (w *Wallet) SetPassphrase(passphrase string) (bool, error) {
	// test that the passphrase was the most recent used
	pubKey, err := w.db.Get([]byte{newestPublicKeyPrefix}, nil)
	if err == leveldb.ErrNotFound {
		w.passphrase = passphrase
		return true, nil
	}
	if err != nil {
		return false, err
	}

	// fetch the private key
	privKeyDbKey, err := encodePrivateKeyDbKey(ed25519.PublicKey(pubKey))
	if err != nil {
		return false, err
	}
	encryptedPrivKey, err := w.db.Get(privKeyDbKey, nil)
	if err != nil {
		return false, err
	}

	// decrypt it
	if _, ok := decryptPrivateKey(encryptedPrivKey, passphrase); !ok {
		return false, nil
	}

	// set it
	w.passphrase = passphrase
	return true, nil
}

// NewKeys generates, encrypts and stores new private keys and returns the public keys.
func (w Wallet) NewKeys(count int) ([]ed25519.PublicKey, error) {
	pubKeys := make([]ed25519.PublicKey, count)
	batch := new(leveldb.Batch)

	for i := 0; i < count; i++ {
		// generate a new key
		pubKey, privKey, err := ed25519.GenerateKey(nil)
		if err != nil {
			return nil, err
		}
		pubKeys[i] = pubKey

		// encrypt the private key
		encryptedPrivKey := encryptPrivateKey(privKey, w.passphrase)
		decryptedPrivKey, ok := decryptPrivateKey(encryptedPrivKey, w.passphrase)

		// safety check
		if !ok || !bytes.Equal(decryptedPrivKey, privKey) {
			return nil, fmt.Errorf("Unable to encrypt/decrypt private keys")
		}

		// store the key
		privKeyDbKey, err := encodePrivateKeyDbKey(pubKey)
		if err != nil {
			return nil, err
		}
		batch.Put(privKeyDbKey, encryptedPrivKey)
		if i+1 == count {
			batch.Put([]byte{newestPublicKeyPrefix}, pubKey)
		}

		// update the filter
		if !w.filter.Insert(pubKey[:]) {
			return nil, fmt.Errorf("Error updating filter")
		}
	}

	wo := opt.WriteOptions{Sync: true}
	if err := w.db.Write(batch, &wo); err != nil {
		return nil, err
	}
	return pubKeys, nil
}

// GetKeys returns all of the public keys from the database.
func (w Wallet) GetKeys() ([]ed25519.PublicKey, error) {
	privKeyDbKey, err := encodePrivateKeyDbKey(nil)
	if err != nil {
		return nil, err
	}
	var pubKeys []ed25519.PublicKey
	iter := w.db.NewIterator(util.BytesPrefix(privKeyDbKey), nil)
	for iter.Next() {
		pubKey, err := decodePrivateKeyDbKey(iter.Key())
		if err != nil {
			iter.Release()
			return nil, err
		}
		pubKeys = append(pubKeys, pubKey)
	}
	iter.Release()
	if err := iter.Error(); err != nil {
		return nil, err
	}
	return pubKeys, nil
}

// Connect connects to a peer for transaction history, balance information, and sending new transactions.
// The threat model assumes the peer the wallet is speaking to is not an adversary.
func (w *Wallet) Connect(addr string, genesisID BlockID) error {
	u := url.URL{Scheme: "wss", Host: addr, Path: "/" + genesisID.String()}
	dialer := websocket.DefaultDialer
	dialer.TLSClientConfig = tlsClientConfig
	dialer.Subprotocols = append(dialer.Subprotocols, Protocol)
	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	w.conn = conn
	w.outChan = make(chan Message)
	w.resultChan = make(chan walletResult, 1)
	return nil
}

// IsConnected returns true if the wallet is connected to a peer.
func (w *Wallet) IsConnected() bool {
	return w.conn != nil
}

// SetTransactionCallback sets a callback to receive new transactions relevant to the wallet.
func (w *Wallet) SetTransactionCallback(callback func(*Transaction)) {
	w.transactionCallback = callback
}

// SetFilterBlockCallback sets a callback to receive new filter blocks with confirmed transactions relevant to this wallet.
func (w *Wallet) SetFilterBlockCallback(callback func(*FilterBlockMessage)) {
	w.filterBlockCallback = callback
}

// GetBalance returns a public key's balance as well as the current block height.
func (w *Wallet) GetBalance(pubKey ed25519.PublicKey) (int64, int64, error) {
	w.outChan <- Message{Type: "get_balance", Body: GetBalanceMessage{PublicKey: pubKey}}
	result := <-w.resultChan
	if len(result.err) != 0 {
		return 0, 0, fmt.Errorf("%s", result.err)
	}
	b := new(BalanceMessage)
	if err := json.Unmarshal(result.message, b); err != nil {
		return 0, 0, err
	}
	return b.Balance, b.Height, nil
}

// GetBalances returns a set of public key balances as well as the current block height.
func (w *Wallet) GetBalances(pubKeys []ed25519.PublicKey) ([]PublicKeyBalance, int64, error) {
	w.outChan <- Message{Type: "get_balances", Body: GetBalancesMessage{PublicKeys: pubKeys}}
	result := <-w.resultChan
	if len(result.err) != 0 {
		return nil, 0, fmt.Errorf("%s", result.err)
	}
	b := new(BalancesMessage)
	if err := json.Unmarshal(result.message, b); err != nil {
		return nil, 0, err
	}
	return b.Balances, b.Height, nil
}

// GetTipHeader returns the current tip of the main chain's header.
func (w *Wallet) GetTipHeader() (BlockID, BlockHeader, error) {
	w.outChan <- Message{Type: "get_tip_header"}
	result := <-w.resultChan
	if len(result.err) != 0 {
		return BlockID{}, BlockHeader{}, fmt.Errorf("%s", result.err)
	}
	th := new(TipHeaderMessage)
	if err := json.Unmarshal(result.message, th); err != nil {
		return BlockID{}, BlockHeader{}, err
	}
	return *th.BlockID, *th.BlockHeader, nil
}

// GetTransactionRelayPolicy returns the peer's transaction relay policy.
func (w *Wallet) GetTransactionRelayPolicy() (minFee, minAmount int64, err error) {
	w.outChan <- Message{Type: "get_transaction_relay_policy"}
	result := <-w.resultChan
	if len(result.err) != 0 {
		return 0, 0, fmt.Errorf("%s", result.err)
	}
	trp := new(TransactionRelayPolicyMessage)
	if err := json.Unmarshal(result.message, trp); err != nil {
		return 0, 0, err
	}
	return trp.MinFee, trp.MinAmount, nil
}

// SetFilter sets the filter for the connection.
func (w *Wallet) SetFilter() error {
	m := Message{
		Type: "filter_load",
		Body: FilterLoadMessage{
			Type:   "cuckoo",
			Filter: w.filter.Encode(),
		},
	}
	w.outChan <- m
	result := <-w.resultChan
	if len(result.err) != 0 {
		return fmt.Errorf("%s", result.err)
	}
	return nil
}

// AddFilter sends a message to add a public key to the filter.
func (w *Wallet) AddFilter(pubKey ed25519.PublicKey) error {
	m := Message{
		Type: "filter_add",
		Body: FilterAddMessage{
			PublicKeys: []ed25519.PublicKey{pubKey},
		},
	}
	w.outChan <- m
	result := <-w.resultChan
	if len(result.err) != 0 {
		return fmt.Errorf("%s", result.err)
	}
	return nil
}

// Send creates, signs and pushes a transaction out to the network.
func (w *Wallet) Send(from, to ed25519.PublicKey, amount, fee, matures, expires int64, memo string) (
	TransactionID, error) {
	// fetch the private key
	privKeyDbKey, err := encodePrivateKeyDbKey(from)
	if err != nil {
		return TransactionID{}, err
	}
	encryptedPrivKey, err := w.db.Get(privKeyDbKey, nil)
	if err != nil {
		return TransactionID{}, err
	}

	// decrypt it
	privKey, ok := decryptPrivateKey(encryptedPrivKey, w.passphrase)
	if !ok {
		return TransactionID{}, fmt.Errorf("Unable to decrypt private key")
	}

	// get the current tip header
	_, header, err := w.GetTipHeader()
	if err != nil {
		return TransactionID{}, err
	}
	// set these relative to the current height
	if matures != 0 {
		matures = header.Height + matures
	}
	if expires != 0 {
		expires = header.Height + expires
	}

	// create the transaction
	tx := NewTransaction(from, to, amount, fee, matures, expires, header.Height, memo)

	// sign it
	if err := tx.Sign(privKey); err != nil {
		return TransactionID{}, err
	}

	// push it
	w.outChan <- Message{Type: "push_transaction", Body: PushTransactionMessage{Transaction: tx}}
	result := <-w.resultChan

	// handle result
	if len(result.err) != 0 {
		return TransactionID{}, fmt.Errorf("%s", result.err)
	}
	ptr := new(PushTransactionResultMessage)
	if err := json.Unmarshal(result.message, ptr); err != nil {
		return TransactionID{}, err
	}
	if len(ptr.Error) != 0 {
		return TransactionID{}, fmt.Errorf("%s", ptr.Error)
	}
	return ptr.TransactionID, nil
}

// GetTransaction retrieves information about a historic transaction.
func (w *Wallet) GetTransaction(id TransactionID) (*Transaction, *BlockID, int64, error) {
	w.outChan <- Message{Type: "get_transaction", Body: GetTransactionMessage{TransactionID: id}}
	result := <-w.resultChan
	if len(result.err) != 0 {
		return nil, nil, 0, fmt.Errorf("%s", result.err)
	}
	t := new(TransactionMessage)
	if err := json.Unmarshal(result.message, t); err != nil {
		return nil, nil, 0, err
	}
	return t.Transaction, t.BlockID, t.Height, nil
}

// Used to hold the result of synchronous requests
type walletResult struct {
	err     string
	message json.RawMessage
}

// Run executes the Wallet's main loop in its own goroutine.
// It manages reading and writing to the peer WebSocket.
func (w *Wallet) Run() {
	w.wg.Add(1)
	go w.run()
}

func (w *Wallet) run() {
	defer w.wg.Done()
	defer func() { w.conn = nil }()
	defer close(w.outChan)

	// writer goroutine loop
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()

		for {
			select {
			case message, ok := <-w.outChan:
				if !ok {
					// channel closed
					return
				}

				// send outgoing message to peer
				if err := w.conn.WriteJSON(message); err != nil {
					w.resultChan <- walletResult{err: err.Error()}
				}
			}
		}
	}()

	// reader loop
	for {
		// new message from peer
		messageType, message, err := w.conn.ReadMessage()
		if err != nil {
			w.resultChan <- walletResult{err: err.Error()}
			break
		}
		switch messageType {
		case websocket.TextMessage:
			var body json.RawMessage
			m := Message{Body: &body}
			if err := json.Unmarshal([]byte(message), &m); err != nil {
				w.resultChan <- walletResult{err: err.Error()}
				break
			}
			switch m.Type {
			case "balance":
				w.resultChan <- walletResult{message: body}

			case "tip_header":
				w.resultChan <- walletResult{message: body}

			case "transaction_relay_policy":
				w.resultChan <- walletResult{message: body}

			case "push_transaction_result":
				w.resultChan <- walletResult{message: body}

			case "transaction":
				w.resultChan <- walletResult{message: body}

			case "filter_result":
				if len(body) != 0 {
					fr := new(FilterResultMessage)
					if err := json.Unmarshal(body, fr); err != nil {
						log.Printf("Error: %s, from: %s\n", err, w.conn.RemoteAddr())
						w.resultChan <- walletResult{err: err.Error()}
						break
					}
					w.resultChan <- walletResult{err: fr.Error}
				} else {
					w.resultChan <- walletResult{}
				}

			case "push_transaction":
				pt := new(PushTransactionMessage)
				if err := json.Unmarshal(body, pt); err != nil {
					log.Printf("Error: %s, from: %s\n", err, w.conn.RemoteAddr())
					break
				}
				if w.transactionCallback != nil {
					w.transactionCallback(pt.Transaction)
				}

			case "filter_block":
				fb := new(FilterBlockMessage)
				if err := json.Unmarshal(body, fb); err != nil {
					log.Printf("Error: %s, from: %s\n", err, w.conn.RemoteAddr())
					break
				}
				if w.filterBlockCallback != nil {
					w.filterBlockCallback(fb)
				}
			}

		case websocket.CloseMessage:
			fmt.Printf("Received close message from: %s\n", w.conn.RemoteAddr())
			break
		}
	}
}

// Shutdown is called to shutdown the wallet synchronously.
func (w *Wallet) Shutdown() error {
	var addr string
	if w.conn != nil {
		addr = w.conn.RemoteAddr().String()
		w.conn.Close()
	}
	w.wg.Wait()
	if len(addr) != 0 {
		log.Printf("Closed connection with %s\n", addr)
	}
	return w.db.Close()
}

// Initialize the filter
func (w *Wallet) initializeFilter() error {
	var capacity int = 4096
	pubKeys, err := w.GetKeys()
	if err != nil {
		return err
	}
	if len(pubKeys) > capacity/2 {
		capacity = len(pubKeys) * 2
	}
	w.filter = cuckoo.NewFilter(uint(capacity))
	for _, pubKey := range pubKeys {
		if !w.filter.Insert(pubKey[:]) {
			return fmt.Errorf("Error building filter")
		}
	}
	return nil
}

// leveldb schema

// n         -> newest public key
// k{pubkey} -> encrypted private key

const newestPublicKeyPrefix = 'n'

const privateKeyPrefix = 'k'

func encodePrivateKeyDbKey(pubKey ed25519.PublicKey) ([]byte, error) {
	key := new(bytes.Buffer)
	if err := key.WriteByte(privateKeyPrefix); err != nil {
		return nil, err
	}
	if err := binary.Write(key, binary.BigEndian, pubKey); err != nil {
		return nil, err
	}
	return key.Bytes(), nil
}

func decodePrivateKeyDbKey(key []byte) (ed25519.PublicKey, error) {
	buf := bytes.NewBuffer(key)
	if _, err := buf.ReadByte(); err != nil {
		return nil, err
	}
	var pubKey [ed25519.PublicKeySize]byte
	if err := binary.Read(buf, binary.BigEndian, pubKey[:32]); err != nil {
		return nil, err
	}
	return ed25519.PublicKey(pubKey[:]), nil
}

// encryption utility functions

// NaCl secretbox encrypt a private key with an Argon2id key derived from passphrase
func encryptPrivateKey(privKey ed25519.PrivateKey, passphrase string) []byte {
	salt := generateSalt()
	key := stretchPassphrase(passphrase, salt)

	var secretKey [32]byte
	copy(secretKey[:], key)

	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		panic(err)
	}

	encrypted := secretbox.Seal(nonce[:], privKey[:], &nonce, &secretKey)

	// prepend the salt
	encryptedPrivKey := make([]byte, len(encrypted)+ArgonSaltLength)
	copy(encryptedPrivKey[:], salt)
	copy(encryptedPrivKey[ArgonSaltLength:], encrypted)

	return encryptedPrivKey
}

// NaCl secretbox decrypt a private key with an Argon2id key derived from passphrase
func decryptPrivateKey(encryptedPrivKey []byte, passphrase string) (ed25519.PrivateKey, bool) {
	salt := encryptedPrivKey[:ArgonSaltLength]
	key := []byte(stretchPassphrase(passphrase, salt))

	var secretKey [32]byte
	copy(secretKey[:], key)

	var nonce [24]byte
	copy(nonce[:], encryptedPrivKey[ArgonSaltLength:ArgonSaltLength+24])

	decryptedPrivKey, ok := secretbox.Open(nil, encryptedPrivKey[ArgonSaltLength+24:], &nonce, &secretKey)
	if !ok {
		return ed25519.PrivateKey{}, false
	}
	return ed25519.PrivateKey(decryptedPrivKey[:]), true
}

const ArgonSaltLength = 16

const ArgonTime = 1

const ArgonMemory = 64 * 1024

const ArgonThreads = 4

const ArgonKeyLength = 32

// Generate a suitable salt for use with Argon2id
func generateSalt() []byte {
	salt := make([]byte, ArgonSaltLength)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		panic(err.Error())
	}
	return salt
}

// Strecth passphrase into a 32 byte key with Argon2id
func stretchPassphrase(passphrase string, salt []byte) []byte {
	return argon2.IDKey([]byte(passphrase), salt, ArgonTime, ArgonMemory, ArgonThreads, ArgonKeyLength)
}
