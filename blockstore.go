// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package p2p

import (
	"context"
	"errors"

	"github.com/ipfs/boxo/blockstore"
	dshelp "github.com/ipfs/boxo/datastore/dshelp"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
	ipld "github.com/ipfs/go-ipld-format"

	"github.com/sourcenetwork/corekv"
)

// Blockstore proxies the ipld.DAGService under the /core namespace for future-proofing
type Blockstore interface {
	blockstore.Blockstore
}

// NewBlockstore returns a default Blockstore implementation
// using the provided datastore.Batching backend.
func newBlockstore(store corekv.ReaderWriter) *bstore {
	return &bstore{
		store: store,
	}
}

type bstore struct {
	store corekv.ReaderWriter

	rehash bool
}

var _ Blockstore = (*bstore)(nil)

// HashOnRead enables or disables rehashing of blocks on read.
func (bs *bstore) HashOnRead(enabled bool) {
	bs.rehash = enabled
}

// Get returns a block from the blockstore.
func (bs *bstore) Get(ctx context.Context, k cid.Cid) (blocks.Block, error) {
	if !k.Defined() {
		return nil, ipld.ErrNotFound{Cid: k}
	}
	bdata, err := bs.store.Get(ctx, dshelp.MultihashToDsKey(k.Hash()).Bytes())
	if errors.Is(err, corekv.ErrNotFound) {
		return nil, ipld.ErrNotFound{Cid: k}
	}
	if err != nil {
		return nil, err
	}
	if bs.rehash {
		rbcid, err := k.Prefix().Sum(bdata)
		if err != nil {
			return nil, err
		}

		if !rbcid.Equals(k) {
			return nil, ErrHashMismatch
		}

		return blocks.NewBlockWithCid(bdata, rbcid)
	}
	return blocks.NewBlockWithCid(bdata, k)
}

// Put stores a block to the blockstore.
func (bs *bstore) Put(ctx context.Context, block blocks.Block) error {
	k := dshelp.MultihashToDsKey(block.Cid().Hash())

	// Has is cheaper than Set, so see if we already have it
	exists, err := bs.store.Has(ctx, k.Bytes())
	if err == nil && exists {
		return nil // already stored.
	}
	return bs.store.Set(ctx, k.Bytes(), block.RawData())
}

// PutMany stores multiple blocks to the blockstore.
func (bs *bstore) PutMany(ctx context.Context, blocks []blocks.Block) error {
	for _, b := range blocks {
		k := dshelp.MultihashToDsKey(b.Cid().Hash())
		exists, err := bs.store.Has(ctx, k.Bytes())
		if err == nil && exists {
			continue
		}

		err = bs.store.Set(ctx, k.Bytes(), b.RawData())
		if err != nil {
			return err
		}
	}
	return nil
}

// Has returns whether a block is stored in the blockstore.
func (bs *bstore) Has(ctx context.Context, k cid.Cid) (bool, error) {
	return bs.store.Has(ctx, dshelp.MultihashToDsKey(k.Hash()).Bytes())
}

// GetSize returns the size of a block in the blockstore.
func (bs *bstore) GetSize(ctx context.Context, k cid.Cid) (int, error) {
	buf, err := bs.store.Get(ctx, dshelp.MultihashToDsKey(k.Hash()).Bytes())
	if errors.Is(err, corekv.ErrNotFound) {
		return -1, ipld.ErrNotFound{Cid: k}
	}
	return len(buf), err
}

// DeleteBlock removes a block from the blockstore.
func (bs *bstore) DeleteBlock(ctx context.Context, k cid.Cid) error {
	return bs.store.Delete(ctx, dshelp.MultihashToDsKey(k.Hash()).Bytes())
}

// AllKeysChan runs a query for keys from the blockstore.
//
// AllKeysChan respects context.
//
// TODO this is very simplistic, in the future, take dsq.Query as a param?
func (bs *bstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	// KeysOnly, because that would be _a lot_ of data.
	iter, err := bs.store.Iterator(ctx, corekv.IterOptions{
		KeysOnly: true,
	})
	if err != nil {
		return nil, err
	}

	output := make(chan cid.Cid, dsq.KeysOnlyBufSize)
	go func() {
		defer func() {
			//nolint:errcheck
			iter.Close() // ensure exit (signals early exit, too)
			close(output)
		}()

		for {
			hasNext, err := iter.Next()
			if err != nil {
				log.ErrorContextE(ctx, "Error iterating through keys", err)
				break
			}

			if !hasNext {
				break
			}

			key := iter.Key()

			hash, err := dshelp.DsKeyToMultihash(ds.RawKey(string(key)))
			if err != nil {
				log.ErrorContextE(ctx, "Error parsing key from binary", err)
				continue
			}
			k := cid.NewCidV1(cid.Raw, hash)
			select {
			case <-ctx.Done():
				return
			case output <- k:
			}
		}
	}()

	return output, nil
}
