// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"errors"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcutil/gcs/builder"
	"github.com/btcsuite/fastsha256"
)

const (
	// cfIndexName is the human-readable name for the index.
	cfIndexName = "committed filter index"
)

// Committed filters come in two flavours: basic and extended. They are
// generated and dropped in pairs, and both are indexed by a block's hash.
// Besides holding different content, they also live in different buckets.
var (
	// cfBasicIndexKey is the name of the db bucket used to house the
	// block hash -> basic cf index (cf#0).
	cfBasicIndexKey = []byte("cf0byhashidx")
	// cfBasicHeaderKey is the name of the db bucket used to house the
	// block hash -> basic cf header index (cf#0).
	cfBasicHeaderKey = []byte("cf0headerbyhashidx")
	// cfExtendedIndexKey is the name of the db bucket used to house the
	// block hash -> extended cf index (cf#1).
	cfExtendedIndexKey = []byte("cf1byhashidx")
	// cfExtendedHeaderKey is the name of the db bucket used to house the
	// block hash -> extended cf header index (cf#1).
	cfExtendedHeaderKey = []byte("cf1headerbyhashidx")
)

// dbFetchFilter retrieves a block's basic or extended filter. A filter's
// absence is not considered an error.
func dbFetchFilter(dbTx database.Tx, key []byte, h *chainhash.Hash) ([]byte, error) {
	idx := dbTx.Metadata().Bucket(key)
	return idx.Get(h[:]), nil
}

// dbFetchFilterHeader retrieves a block's basic or extended filter header.
// A filter's absence is not considered an error.
func dbFetchFilterHeader(dbTx database.Tx, key []byte, h *chainhash.Hash) ([]byte, error) {
	idx := dbTx.Metadata().Bucket(key)
	fh := idx.Get(h[:])
	if len(fh) != fastsha256.Size {
		return nil, errors.New("invalid filter header length")
	}
	return fh, nil
}

// dbStoreFilter stores a block's basic or extended filter.
func dbStoreFilter(dbTx database.Tx, key []byte, h *chainhash.Hash, f []byte) error {
	idx := dbTx.Metadata().Bucket(key)
	return idx.Put(h[:], f)
}

// dbStoreFilterHeader stores a block's basic or extended filter header.
func dbStoreFilterHeader(dbTx database.Tx, key []byte, h *chainhash.Hash, fh []byte) error {
	if len(fh) != fastsha256.Size {
		return errors.New("invalid filter header length")
	}
	idx := dbTx.Metadata().Bucket(key)
	return idx.Put(h[:], fh)
}

// dbDeleteFilter deletes a filter's basic or extended filter.
func dbDeleteFilter(dbTx database.Tx, key []byte, h *chainhash.Hash) error {
	idx := dbTx.Metadata().Bucket(key)
	return idx.Delete(h[:])
}

// dbDeleteFilterHeader deletes a filter's basic or extended filter header.
func dbDeleteFilterHeader(dbTx database.Tx, key []byte, h *chainhash.Hash) error {
	idx := dbTx.Metadata().Bucket(key)
	return idx.Delete(h[:])
}

// CfIndex implements a committed filter (cf) by hash index.
type CfIndex struct {
	db          database.DB
	chainParams *chaincfg.Params
}

// Ensure the CfIndex type implements the Indexer interface.
var _ Indexer = (*CfIndex)(nil)

// Init initializes the hash-based cf index. This is part of the Indexer
// interface.
func (idx *CfIndex) Init() error {
	return nil // Nothing to do.
}

// Key returns the database key to use for the index as a byte slice. This is
// part of the Indexer interface.
func (idx *CfIndex) Key() []byte {
	return cfBasicIndexKey
}

// Name returns the human-readable name of the index. This is part of the
// Indexer interface.
func (idx *CfIndex) Name() string {
	return cfIndexName
}

// Create is invoked when the indexer manager determines the index needs to
// be created for the first time. It creates buckets for the two hash-based cf
// indexes (simple, extended).
func (idx *CfIndex) Create(dbTx database.Tx) error {
	meta := dbTx.Metadata()
	_, err := meta.CreateBucket(cfBasicIndexKey)
	if err != nil {
		return err
	}
	_, err = meta.CreateBucket(cfBasicHeaderKey)
	if err != nil {
		return err
	}
	_, err = meta.CreateBucket(cfExtendedIndexKey)
	if err != nil {
		return err
	}
	_, err = meta.CreateBucket(cfExtendedHeaderKey)
	if err != nil {
		return err
	}
	firstHeader := make([]byte, chainhash.HashSize)
	err = dbStoreFilterHeader(
		dbTx,
		cfBasicHeaderKey,
		&idx.chainParams.GenesisBlock.Header.PrevBlock,
		firstHeader,
	)
	if err != nil {
		return err
	}
	err = dbStoreFilterHeader(
		dbTx,
		cfExtendedHeaderKey,
		&idx.chainParams.GenesisBlock.Header.PrevBlock,
		firstHeader,
	)
	return err
}

// makeBasicFilterForBlock builds a block's basic filter, which consists of
// all outpoints and pkscript data pushes referenced by transactions within the
// block.
func makeBasicFilterForBlock(block *btcutil.Block) ([]byte, error) {
	b := builder.WithKeyHash(block.Hash())
	_, err := b.Key()
	if err != nil {
		return nil, err
	}
	for i, tx := range block.Transactions() {
		// Skip the inputs for the coinbase transaction
		if i != 0 {
			for _, txIn := range tx.MsgTx().TxIn {
				b.AddOutPoint(txIn.PreviousOutPoint)
			}
		}
		for _, txOut := range tx.MsgTx().TxOut {
			b.AddScript(txOut.PkScript)
		}
	}
	f, err := b.Build()
	if err != nil {
		return nil, err
	}
	return f.Bytes(), nil
}

// makeExtendedFilterForBlock builds a block's extended filter, which consists
// of all tx hashes and sigscript data pushes contained in the block.
func makeExtendedFilterForBlock(block *btcutil.Block) ([]byte, error) {
	b := builder.WithKeyHash(block.Hash())
	_, err := b.Key()
	if err != nil {
		return nil, err
	}
	for i, tx := range block.Transactions() {
		b.AddHash(tx.Hash())
		// Skip the inputs for the coinbase transaction
		if i != 0 {
			for _, txIn := range tx.MsgTx().TxIn {
				b.AddScript(txIn.SignatureScript)
			}
		}
	}
	f, err := b.Build()
	if err != nil {
		return nil, err
	}
	return f.Bytes(), nil
}

// makeHeaderForFilter implements the chaining logic between filters, where
// a filter's header is defined as sha256(sha256(filter) + previousFilterHeader).
func makeHeaderForFilter(f, pfh []byte) []byte {
	fhash := fastsha256.Sum256(f)
	chain := make([]byte, 0, 2*fastsha256.Size)
	chain = append(chain, fhash[:]...)
	chain = append(chain, pfh...)
	fh := fastsha256.Sum256(chain)
	return fh[:]
}

// storeFilter stores a given filter, and performs the steps needed to
// generate the filter's header.
func storeFilter(dbTx database.Tx, block *btcutil.Block, f []byte, extended bool) error {
	// Figure out which buckets to use.
	fkey := cfBasicIndexKey
	hkey := cfBasicHeaderKey
	if extended {
		fkey = cfExtendedIndexKey
		hkey = cfExtendedHeaderKey
	}
	// Start by storing the filter.
	h := block.Hash()
	err := dbStoreFilter(dbTx, fkey, h, f)
	if err != nil {
		return err
	}
	// Then fetch the previous block's filter header.
	ph := &block.MsgBlock().Header.PrevBlock
	pfh, err := dbFetchFilterHeader(dbTx, hkey, ph)
	if err != nil {
		return err
	}
	// Construct the new block's filter header, and store it.
	fh := makeHeaderForFilter(f, pfh)
	return dbStoreFilterHeader(dbTx, hkey, h, fh)
}

// ConnectBlock is invoked by the index manager when a new block has been
// connected to the main chain. This indexer adds a hash-to-cf mapping for
// every passed block. This is part of the Indexer interface.
func (idx *CfIndex) ConnectBlock(dbTx database.Tx, block *btcutil.Block,
	view *blockchain.UtxoViewpoint) error {
	f, err := makeBasicFilterForBlock(block)
	if err != nil {
		return err
	}
	err = storeFilter(dbTx, block, f, false)
	if err != nil {
		return err
	}
	f, err = makeExtendedFilterForBlock(block)
	if err != nil {
		return err
	}
	return storeFilter(dbTx, block, f, true)
}

// DisconnectBlock is invoked by the index manager when a block has been
// disconnected from the main chain.  This indexer removes the hash-to-cf
// mapping for every passed block. This is part of the Indexer interface.
func (idx *CfIndex) DisconnectBlock(dbTx database.Tx, block *btcutil.Block,
	view *blockchain.UtxoViewpoint) error {
	err := dbDeleteFilter(dbTx, cfBasicIndexKey, block.Hash())
	if err != nil {
		return err
	}
	return dbDeleteFilter(dbTx, cfExtendedIndexKey, block.Hash())
}

// FilterByBlockHash returns the serialized contents of a block's basic or
// extended committed filter.
func (idx *CfIndex) FilterByBlockHash(h *chainhash.Hash, extended bool) ([]byte, error) {
	var f []byte
	err := idx.db.View(func(dbTx database.Tx) error {
		var err error
		key := cfBasicIndexKey
		if extended {
			key = cfExtendedIndexKey
		}
		f, err = dbFetchFilter(dbTx, key, h)
		return err
	})
	return f, err
}

// FilterHeaderByBlockHash returns the serialized contents of a block's basic
// or extended committed filter header.
func (idx *CfIndex) FilterHeaderByBlockHash(h *chainhash.Hash, extended bool) ([]byte, error) {
	var fh []byte
	err := idx.db.View(func(dbTx database.Tx) error {
		var err error
		key := cfBasicHeaderKey
		if extended {
			key = cfExtendedHeaderKey
		}
		fh, err = dbFetchFilterHeader(dbTx, key, h)
		return err
	})
	return fh, err
}

// NewCfIndex returns a new instance of an indexer that is used to create a
// mapping of the hashes of all blocks in the blockchain to their respective
// committed filters.
//
// It implements the Indexer interface which plugs into the IndexManager that in
// turn is used by the blockchain package. This allows the index to be
// seamlessly maintained along with the chain.
func NewCfIndex(db database.DB, chainParams *chaincfg.Params) *CfIndex {
	return &CfIndex{db: db, chainParams: chainParams}
}

// DropCfIndex drops the CF index from the provided database if exists.
func DropCfIndex(db database.DB) error {
	err := dropIndex(db, cfBasicIndexKey, cfIndexName)
	if err != nil {
		return err
	}
	err = dropIndex(db, cfBasicHeaderKey, cfIndexName)
	if err != nil {
		return err
	}
	err = dropIndex(db, cfExtendedIndexKey, cfIndexName)
	if err != nil {
		return err
	}
	err = dropIndex(db, cfExtendedHeaderKey, cfIndexName)
	return err
}