// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"github.com/buger/jsonparser"
	"github.com/pierrec/lz4"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// BlockStorageDisk is an on-disk BlockStorage implementation using the filesystem for blocks
// and LevelDB for block headers.
type BlockStorageDisk struct {
	db       *leveldb.DB
	dirPath  string
	readOnly bool
	compress bool
}

// NewBlockStorageDisk returns a new instance of on-disk block storage.
func NewBlockStorageDisk(dirPath, dbPath string, readOnly, compress bool) (*BlockStorageDisk, error) {
	// create the blocks path if it doesn't exist
	if !readOnly {
		if info, err := os.Stat(dirPath); os.IsNotExist(err) {
			if err := os.MkdirAll(dirPath, 0700); err != nil {
				return nil, err
			}
		} else if !info.IsDir() {
			return nil, fmt.Errorf("%s is not a directory", dirPath)
		}
	}

	// open the database
	opts := opt.Options{ReadOnly: readOnly}
	db, err := leveldb.OpenFile(dbPath, &opts)
	if err != nil {
		return nil, err
	}
	return &BlockStorageDisk{
		db:       db,
		dirPath:  dirPath,
		readOnly: readOnly,
		compress: compress,
	}, nil
}

// Store is called to store all of the block's information.
func (b BlockStorageDisk) Store(id BlockID, block *Block, now int64) error {
	if b.readOnly {
		return fmt.Errorf("Block storage is in read-only mode")
	}

	// save the complete block to the filesystem
	blockBytes, err := json.Marshal(block)
	if err != nil {
		return err
	}

	var ext string
	if b.compress {
		// compress with lz4
		in := bytes.NewReader(blockBytes)
		zout := new(bytes.Buffer)
		zw := lz4.NewWriter(zout)
		if _, err := io.Copy(zw, in); err != nil {
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
		blockBytes = zout.Bytes()
		ext = ".lz4"
	} else {
		ext = ".json"
	}

	// write the block and sync
	blockPath := filepath.Join(b.dirPath, id.String()+ext)
	f, err := os.OpenFile(blockPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	n, err := f.Write(blockBytes)
	if err != nil {
		return err
	}
	if err == nil && n < len(blockBytes) {
		return io.ErrShortWrite
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	// save the header to leveldb
	encodedBlockHeader, err := encodeBlockHeader(block.Header, now)
	if err != nil {
		return err
	}

	wo := opt.WriteOptions{Sync: true}
	return b.db.Put(id[:], encodedBlockHeader, &wo)
}

// Get returns the referenced block.
func (b BlockStorageDisk) GetBlock(id BlockID) (*Block, error) {
	blockJson, err := b.GetBlockBytes(id)
	if err != nil {
		return nil, err
	}

	// unmarshal
	block := new(Block)
	if err := json.Unmarshal(blockJson, block); err != nil {
		return nil, err
	}
	return block, nil
}

// GetBlockBytes returns the referenced block as a byte slice.
func (b BlockStorageDisk) GetBlockBytes(id BlockID) ([]byte, error) {
	var ext [2]string
	if b.compress {
		// order to try finding the block by extension
		ext = [2]string{".lz4", ".json"}
	} else {
		ext = [2]string{".json", ".lz4"}
	}

	var compressed bool = b.compress

	blockPath := filepath.Join(b.dirPath, id.String()+ext[0])
	if _, err := os.Stat(blockPath); os.IsNotExist(err) {
		compressed = !compressed
		blockPath = filepath.Join(b.dirPath, id.String()+ext[1])
		if _, err := os.Stat(blockPath); os.IsNotExist(err) {
			// not found
			return nil, nil
		}
	}

	// read it off disk
	blockBytes, err := ioutil.ReadFile(blockPath)
	if err != nil {
		return nil, err
	}

	if compressed {
		// uncompress
		zin := bytes.NewBuffer(blockBytes)
		out := new(bytes.Buffer)
		zr := lz4.NewReader(zin)
		if _, err := io.Copy(out, zr); err != nil {
			return nil, err
		}
		blockBytes = out.Bytes()
	}

	return blockBytes, nil
}

// GetBlockHeader returns the referenced block's header and the timestamp of when it was stored.
func (b BlockStorageDisk) GetBlockHeader(id BlockID) (*BlockHeader, int64, error) {
	// fetch it
	encodedHeader, err := b.db.Get(id[:], nil)
	if err == leveldb.ErrNotFound {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}

	// decode it
	return decodeBlockHeader(encodedHeader)
}

// GetTransaction returns a transaction within a block and the block's header.
func (b BlockStorageDisk) GetTransaction(id BlockID, index int) (
	*Transaction, *BlockHeader, error) {
	blockJson, err := b.GetBlockBytes(id)
	if err != nil {
		return nil, nil, err
	}

	// pick out and unmarshal the transaction at the index
	idx := "[" + strconv.Itoa(index) + "]"
	txJson, _, _, err := jsonparser.Get(blockJson, "transactions", idx)
	if err != nil {
		return nil, nil, err
	}
	tx := new(Transaction)
	if err := json.Unmarshal(txJson, tx); err != nil {
		return nil, nil, err
	}

	// pick out and unmarshal the header
	hdrJson, _, _, err := jsonparser.Get(blockJson, "header")
	if err != nil {
		return nil, nil, err
	}
	header := new(BlockHeader)
	if err := json.Unmarshal(hdrJson, header); err != nil {
		return nil, nil, err
	}
	return tx, header, nil
}

// Close is called to close any underlying storage.
func (b *BlockStorageDisk) Close() error {
	return b.db.Close()
}

// leveldb schema: {bid} -> {timestamp}{gob encoded header}

func encodeBlockHeader(header *BlockHeader, when int64) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, when); err != nil {
		return nil, err
	}
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(header); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeBlockHeader(encodedHeader []byte) (*BlockHeader, int64, error) {
	buf := bytes.NewBuffer(encodedHeader)
	var when int64
	if err := binary.Read(buf, binary.BigEndian, &when); err != nil {
		return nil, 0, err
	}
	enc := gob.NewDecoder(buf)
	header := new(BlockHeader)
	if err := enc.Decode(header); err != nil {
		return nil, 0, err
	}
	return header, when, nil
}
