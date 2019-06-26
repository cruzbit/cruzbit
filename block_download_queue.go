// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"container/list"
	"sync"
	"time"
)

// BlockDownloadQueue is a queue of block IDs to download.
type BlockDownloadQueue struct {
	blockMap   map[BlockID]*list.Element
	blockQueue *list.List
	lock       sync.RWMutex
}

// If a block has been in the queue for more than 2 minutes it can be re-added with a new peer
// responsible for its download.
const maxQueueWait = 2 * time.Minute

type blockDownloadQueueEntry struct {
	id   BlockID
	who  string
	when time.Time
}

// NewBlockDownloadQueue returns a new instance of a BlockDownloadQueue.
func NewBlockDownloadQueue() *BlockDownloadQueue {
	return &BlockDownloadQueue{
		blockMap:   make(map[BlockID]*list.Element),
		blockQueue: list.New(),
	}
}

// PushBack adds the block ID to the back of the queue and records the address of the peer who pushed it.
func (b *BlockDownloadQueue) PushBack(id BlockID, who string) bool {
	b.lock.Lock()
	defer b.lock.Unlock()
	if e, ok := b.blockMap[id]; ok {
		entry := e.Value.(*blockDownloadQueueEntry)
		if time.Since(entry.when) < maxQueueWait {
			// it's still pending download
			return false
		}
		// it's expired. signal that it can be tried again and leave it in place
		entry.when = time.Now()
		// new peer owns its place in the queue
		entry.who = who
		return true
	}

	// add to the back of the queue
	entry := &blockDownloadQueueEntry{id: id, who: who, when: time.Now()}
	e := b.blockQueue.PushBack(entry)
	b.blockMap[id] = e
	return true
}

// Remove removes the block ID from the queue only if the requester is who added it.
func (b *BlockDownloadQueue) Remove(id BlockID, who string) bool {
	b.lock.Lock()
	defer b.lock.Unlock()
	if e, ok := b.blockMap[id]; ok {
		entry := e.Value.(*blockDownloadQueueEntry)
		if entry.who == who {
			b.blockQueue.Remove(e)
			delete(b.blockMap, entry.id)
			return true
		}
	}
	return false
}

// Exists returns true if the block ID exists in the queue.
func (b *BlockDownloadQueue) Exists(id BlockID) bool {
	b.lock.RLock()
	defer b.lock.RUnlock()
	_, ok := b.blockMap[id]
	return ok
}

// PeekFront returns the ID of the block at the front of the queue.
func (b *BlockDownloadQueue) PeekFront() (BlockID, bool) {
	b.lock.RLock()
	defer b.lock.RUnlock()
	if b.blockQueue.Len() == 0 {
		return BlockID{}, false
	}
	e := b.blockQueue.Front()
	entry := e.Value.(*blockDownloadQueueEntry)
	return entry.id, true
}

// Len returns the length of the queue.
func (b *BlockDownloadQueue) Len() int {
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.blockQueue.Len()
}
