// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"container/list"
	"sync"
	"time"
)

// BlockQueue is a queue of blocks to download.
type BlockQueue struct {
	blockMap   map[BlockID]*list.Element
	blockQueue *list.List
	lock       sync.RWMutex
}

// If a block has been in the queue for more than 2 minutes it can be re-added with a new peer
// responsible for its download.
const maxQueueWait = 2 * time.Minute

type blockQueueEntry struct {
	id   BlockID
	who  string
	when time.Time
}

// NewBlockQueue returns a new instance of a BlockQueue.
func NewBlockQueue() *BlockQueue {
	return &BlockQueue{
		blockMap:   make(map[BlockID]*list.Element),
		blockQueue: list.New(),
	}
}

// PushBack adds the block ID to the back of the queue and records the address of the peer who pushed it if it didn't exist in the queue.
// If it did exist and maxQueueWait has elapsed, the block is left in its position but the peer responsible for download is updated.
func (b *BlockQueue) PushBack(id BlockID, who string) bool {
	b.lock.Lock()
	defer b.lock.Unlock()
	if e, ok := b.blockMap[id]; ok {
		entry := e.Value.(*blockQueueEntry)
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
	entry := &blockQueueEntry{id: id, who: who, when: time.Now()}
	e := b.blockQueue.PushBack(entry)
	b.blockMap[id] = e
	return true
}

// Remove removes the block ID from the queue only if the requester is who is currently responsible for its download.
func (b *BlockQueue) Remove(id BlockID, who string) bool {
	b.lock.Lock()
	defer b.lock.Unlock()
	if e, ok := b.blockMap[id]; ok {
		entry := e.Value.(*blockQueueEntry)
		if entry.who == who {
			b.blockQueue.Remove(e)
			delete(b.blockMap, entry.id)
			return true
		}
	}
	return false
}

// Exists returns true if the block ID exists in the queue.
func (b *BlockQueue) Exists(id BlockID) bool {
	b.lock.RLock()
	defer b.lock.RUnlock()
	_, ok := b.blockMap[id]
	return ok
}

// PeekFront returns the ID of the block at the front of the queue.
func (b *BlockQueue) PeekFront() (BlockID, bool) {
	b.lock.RLock()
	defer b.lock.RUnlock()
	if b.blockQueue.Len() == 0 {
		return BlockID{}, false
	}
	e := b.blockQueue.Front()
	entry := e.Value.(*blockQueueEntry)
	return entry.id, true
}

// Len returns the length of the queue.
func (b *BlockQueue) Len() int {
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.blockQueue.Len()
}
