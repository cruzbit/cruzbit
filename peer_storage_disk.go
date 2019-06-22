// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// PeerStorageDisk is an on-disk implementation of the PeerStorage interface using LevelDB.
type PeerStorageDisk struct {
	db                 *leveldb.DB
	connectedPeers     map[string]bool
	connectedPeersLock sync.Mutex
}

// NewPeerStorageDisk returns a new PeerStorageDisk instance.
func NewPeerStorageDisk(dbPath string) (*PeerStorageDisk, error) {
	// open peer database
	db, err := leveldb.OpenFile(dbPath, nil)
	if err != nil {
		return nil, err
	}

	return &PeerStorageDisk{db: db, connectedPeers: make(map[string]bool)}, nil
}

// Store stores a peer address.
func (p *PeerStorageDisk) Store(addr string) error {
	// do we know about it already?
	pi, err := getPeerInfo(addr, p.db)
	if err != nil {
		return err
	}
	if pi != nil {
		// we've seen it
		return nil
	}

	info := peerInfo{FirstSeen: time.Now().Unix()}
	batch := new(leveldb.Batch)
	if err := info.writeToBatch(addr, batch); err != nil {
		return err
	}

	// compute last attempt by time db key
	attemptKey, err := computeLastAttemptTimeKey(info.LastAttempt, addr)
	if err != nil {
		return err
	}
	batch.Put(attemptKey, []byte{0x1})

	// write the batch
	return p.db.Write(batch, nil)
}

// Get returns some peers for us to attempt to connect to.
func (p *PeerStorageDisk) Get(count int) ([]string, error) {
	startKey, err := computeLastAttemptTimeKey(0, "")
	if err != nil {
		return nil, err
	}

	endKey, err := computeLastAttemptTimeKey(time.Now().Unix(), "")
	if err != nil {
		return nil, err
	}

	snapshot, err := p.db.GetSnapshot()
	defer snapshot.Release()
	if err != nil {
		return nil, err
	}

	var addrs []string

	connectedPeers := p.getConnectedPeers()

	// try finding peers
	iter := snapshot.NewIterator(&util.Range{Start: startKey, Limit: endKey}, nil)
	for iter.Next() {
		_, addr, err := decodeLastAttemptTimeKey(iter.Key())
		if err != nil {
			iter.Release()
			return nil, err
		}
		if _, ok := connectedPeers[addr]; ok {
			// already connected
			continue
		}

		// is it time to retry this address?
		info, err := getPeerInfo(addr, snapshot)
		if err != nil {
			iter.Release()
			return nil, err
		}
		if !info.shouldRetry() {
			continue
		}

		// add it to the list
		addrs = append(addrs, addr)
		if len(addrs) == count {
			break
		}
	}
	iter.Release()
	if err := iter.Error(); err != nil {
		return nil, err
	}

	return addrs, nil
}

// GetSince returns some peers to tell others about last active less than "when" ago.
func (p *PeerStorageDisk) GetSince(count int, when int64) ([]string, error) {
	startKey, err := computeLastSuccessTimeKey(when, "")
	if err != nil {
		return nil, err
	}

	endKey, err := computeLastSuccessTimeKey(time.Now().Unix(), "")
	if err != nil {
		return nil, err
	}

	snapshot, err := p.db.GetSnapshot()
	defer snapshot.Release()
	if err != nil {
		return nil, err
	}

	var addrs []string

	// try finding peers
	iter := snapshot.NewIterator(&util.Range{Start: startKey, Limit: endKey}, nil)
	for ok := iter.Last(); ok; ok = iter.Prev() {
		_, addr, err := decodeLastSuccessTimeKey(iter.Key())
		if err != nil {
			iter.Release()
			return nil, err
		}
		// add it to the list
		addrs = append(addrs, addr)
		if len(addrs) == count {
			break
		}
	}
	iter.Release()
	if err := iter.Error(); err != nil {
		return nil, err
	}

	return addrs, nil
}

// OnConnectAttempt is called prior to attempting to connect to the peer.
func (p *PeerStorageDisk) OnConnectAttempt(addr string) error {
	info, err := getPeerInfo(addr, p.db)
	if err != nil {
		return err
	}
	if info == nil {
		return fmt.Errorf("Peer info not found for: %s", addr)
	}

	batch := new(leveldb.Batch)

	// delete last attempt by time entry
	attemptKeyOld, err := computeLastAttemptTimeKey(info.LastAttempt, addr)
	if err != nil {
		return err
	}
	batch.Delete(attemptKeyOld)

	// update last attempt
	info.LastAttempt = time.Now().Unix()
	if err := info.writeToBatch(addr, batch); err != nil {
		return err
	}

	// compute new last attempt by time db key
	attemptKeyNew, err := computeLastAttemptTimeKey(info.LastAttempt, addr)
	if err != nil {
		return err
	}
	batch.Put(attemptKeyNew, []byte{0x1})

	// write the batch
	return p.db.Write(batch, nil)
}

// OnConnectFailure is called upon connection failure.
func (p *PeerStorageDisk) OnConnectFailure(addr string) error {
	info, err := getPeerInfo(addr, p.db)
	if err != nil {
		return err
	}
	if info == nil {
		return fmt.Errorf("Peer info not found for: %s", addr)
	}

	if info.shouldDelete() {
		return p.deletePeer(addr, info.LastAttempt, info.LastSuccess)
	}

	return nil
}

// OnConnectSuccess is called upon successful handshake with the peer.
func (p *PeerStorageDisk) OnConnectSuccess(addr string) error {
	info, err := getPeerInfo(addr, p.db)
	if err != nil {
		return err
	}
	if info == nil {
		return fmt.Errorf("Peer info not found for: %s", addr)
	}

	batch := new(leveldb.Batch)

	// delete last success by time entry
	successKeyOld, err := computeLastSuccessTimeKey(info.LastSuccess, addr)
	if err != nil {
		return err
	}
	batch.Delete(successKeyOld)

	// update last success
	info.LastSuccess = time.Now().Unix()
	if err := info.writeToBatch(addr, batch); err != nil {
		return err
	}

	// compute new success attempt by time db key
	successKeyNew, err := computeLastSuccessTimeKey(info.LastSuccess, addr)
	if err != nil {
		return err
	}
	batch.Put(successKeyNew, []byte{0x1})

	// write the batch
	if err := p.db.Write(batch, nil); err != nil {
		return err
	}

	// save the connected status in memory
	p.connectedPeersLock.Lock()
	defer p.connectedPeersLock.Unlock()
	p.connectedPeers[addr] = true
	return nil
}

// OnDisconnect is called upon disconnection.
func (p *PeerStorageDisk) OnDisconnect(addr string) error {
	p.connectedPeersLock.Lock()
	defer p.connectedPeersLock.Unlock()
	delete(p.connectedPeers, addr)
	return nil
}

// Close is called to close any underlying storage.
func (p *PeerStorageDisk) Close() error {
	return p.db.Close()
}

// Helper to lookup peer info
func getPeerInfo(addr string, db leveldb.Reader) (*peerInfo, error) {
	key, err := computePeerKey(addr)
	if err != nil {
		return nil, err
	}

	encoded, err := db.Get(key, nil)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}

	info := new(peerInfo)
	if err := decodePeerInfo(encoded, info); err != nil {
		return nil, err
	}
	return info, nil
}

// Helper to delete a peer
func (p *PeerStorageDisk) deletePeer(addr string, lastAttempt, lastSuccess int64) error {
	peerKey, err := computePeerKey(addr)
	if err != nil {
		return err
	}
	attemptKey, err := computeLastAttemptTimeKey(lastAttempt, addr)
	if err != nil {
		return err
	}
	successKey, err := computeLastSuccessTimeKey(lastSuccess, addr)
	if err != nil {
		return err
	}
	batch := new(leveldb.Batch)
	batch.Delete(peerKey)
	batch.Delete(attemptKey)
	batch.Delete(successKey)
	return p.db.Write(batch, nil)
}

// Helper to return a copy of the connected set
func (p *PeerStorageDisk) getConnectedPeers() map[string]bool {
	// copy the set of connected peers
	connectedPeers := make(map[string]bool)
	p.connectedPeersLock.Lock()
	defer p.connectedPeersLock.Unlock()
	for addr, _ := range p.connectedPeers {
		connectedPeers[addr] = true
	}
	return connectedPeers
}

// leveldb schema

// p{addr}       -> serialized peerInfo
// a{time}{addr} -> 1 (time is of last attempt)
// s{time}{addr} -> 1 (time is of last success)

const peerPrefix = 'p'

const peerLastAttemptTimePrefix = 'a'

const peerLastSuccessTimePrefix = 's'

type peerInfo struct {
	FirstSeen   int64
	LastAttempt int64
	LastSuccess int64
}

// Should we retry this connection?
func (p peerInfo) shouldRetry() bool {
	if p.LastAttempt == 0 {
		// never been tried
		return true
	}
	lastSeen := p.LastSuccess
	if lastSeen == 0 {
		// never successfully connected, go by first seen
		lastSeen = p.FirstSeen
	}

	now := time.Now().Unix()
	hoursSinceLastSeen := (now - lastSeen) / (60 * 60)
	minutesSinceLastAttempt := (now - p.LastAttempt) / 60
	hoursSinceLastAttempt := minutesSinceLastAttempt / 60

	if hoursSinceLastSeen <= 0 {
		return minutesSinceLastAttempt > 10
	}

	retryInterval := int64(math.Ceil(math.Sqrt(float64(hoursSinceLastSeen))))
	return hoursSinceLastAttempt > retryInterval
}

// Should we delete this peer?
func (p peerInfo) shouldDelete() bool {
	if p.LastSuccess == 0 {
		// if we fail connecting on the first try delete it
		return true
	}
	// has it been over a week since we connected to it?
	lastSuccess := time.Unix(p.LastSuccess, 0)
	return time.Since(lastSuccess) > time.Duration(24*7*time.Hour)
}

// Helper to write the peer info to a batch
func (p peerInfo) writeToBatch(addr string, batch *leveldb.Batch) error {
	key, err := computePeerKey(addr)
	if err != nil {
		return err
	}
	encoded, err := encodePeerInfo(p)
	if err != nil {
		return err
	}
	batch.Put(key, encoded)
	return nil
}

// Encode time as bytes
func encodeTime(when int64) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, when); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Decode time from bytes
func decodeTime(timeBytes []byte) (int64, error) {
	buf := bytes.NewBuffer(timeBytes)
	var when int64
	if err := binary.Read(buf, binary.BigEndian, &when); err != nil {
		return 0, err
	}
	return when, nil
}

func computePeerKey(addr string) ([]byte, error) {
	key := new(bytes.Buffer)
	if err := key.WriteByte(peerPrefix); err != nil {
		return nil, err
	}
	if _, err := key.WriteString(addr); err != nil {
		return nil, err
	}
	return key.Bytes(), nil
}

func computeLastAttemptTimeKey(when int64, addr string) ([]byte, error) {
	key := new(bytes.Buffer)
	if err := key.WriteByte(peerLastAttemptTimePrefix); err != nil {
		return nil, err
	}
	if err := binary.Write(key, binary.BigEndian, when); err != nil {
		return nil, err
	}
	if _, err := key.WriteString(addr); err != nil {
		return nil, err
	}
	return key.Bytes(), nil
}

func decodeLastAttemptTimeKey(key []byte) (int64, string, error) {
	buf := bytes.NewBuffer(key)
	if _, err := buf.ReadByte(); err != nil {
		return 0, "", err
	}
	var when int64
	if err := binary.Read(buf, binary.BigEndian, &when); err != nil {
		return 0, "", err
	}
	return when, buf.String(), nil
}

func computeLastSuccessTimeKey(when int64, addr string) ([]byte, error) {
	key := new(bytes.Buffer)
	if err := key.WriteByte(peerLastSuccessTimePrefix); err != nil {
		return nil, err
	}
	if err := binary.Write(key, binary.BigEndian, when); err != nil {
		return nil, err
	}
	if _, err := key.WriteString(addr); err != nil {
		return nil, err
	}
	return key.Bytes(), nil
}

func decodeLastSuccessTimeKey(key []byte) (int64, string, error) {
	buf := bytes.NewBuffer(key)
	if _, err := buf.ReadByte(); err != nil {
		return 0, "", err
	}
	var when int64
	if err := binary.Read(buf, binary.BigEndian, &when); err != nil {
		return 0, "", err
	}
	return when, buf.String(), nil
}

func encodePeerInfo(info peerInfo) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(&info); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodePeerInfo(encoded []byte, info *peerInfo) error {
	buf := bytes.NewBuffer(encoded)
	enc := gob.NewDecoder(buf)
	return enc.Decode(info)
}
