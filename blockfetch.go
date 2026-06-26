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
	"fmt"
	"time"

	"github.com/fxamacker/cbor/v2"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/storage"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// blockFetchProtocol is the libp2p protocol ID under which a Peer answers
// content-addressed block fetches.
const blockFetchProtocol protocol.ID = "/sourcenetwork/blockfetch/1.0.0"

// blockFetchPerPeerTimeout caps how long we wait for any single peer to answer a fetch.
//
// A short timeout keeps a slow or unresponsive peer from blocking the entire DAG walk.
const blockFetchPerPeerTimeout = 5 * time.Second

type blockFetchStatus uint8

const (
	blockFetchStatusOK       blockFetchStatus = 0
	blockFetchStatusNotFound blockFetchStatus = 1
	blockFetchStatusDenied   blockFetchStatus = 2
)

// blockFetchRequest asks a remote peer for a single block by CID.
type blockFetchRequest struct {
	CID []byte `cbor:"c"`
}

// blockFetchReply carries the response for a blockFetchRequest.
type blockFetchReply struct {
	Status blockFetchStatus `cbor:"s"`
	Data   []byte           `cbor:"d,omitempty"`
}

// sessionKey is the context key under which a fetch session stores its candidate peer IDs.
type sessionKey struct{}

func sessionPeers(ctx context.Context) []string {
	v, _ := ctx.Value(sessionKey{}).([]string)
	return v
}

// handleBlockFetch is the libp2p stream handler for inbound block requests.
//
// One request, one reply, then close. Access decisions defer to the peer's
// configured BlockAccessFunc (if any); a missing func means open access, which
// matches the bitswap-era default.
func (p *Peer) handleBlockFetch(s network.Stream) {
	defer s.Close()

	var req blockFetchRequest
	if err := cbor.NewDecoder(s).Decode(&req); err != nil {
		_ = s.Reset()
		return
	}

	c, err := cid.Cast(req.CID)
	if err != nil {
		_ = cbor.NewEncoder(s).Encode(blockFetchReply{Status: blockFetchStatusNotFound})
		return
	}

	if !p.hasAccess(s.Conn().RemotePeer(), c) {
		_ = cbor.NewEncoder(s).Encode(blockFetchReply{Status: blockFetchStatusDenied})
		return
	}

	block, err := p.blockstore.Get(p.ctx, c)
	if err != nil {
		_ = cbor.NewEncoder(s).Encode(blockFetchReply{Status: blockFetchStatusNotFound})
		return
	}

	_ = cbor.NewEncoder(s).Encode(blockFetchReply{
		Status: blockFetchStatusOK,
		Data:   block.RawData(),
	})
}

// fetchBlockFromPeer opens a stream, sends a single CID request, and reads the reply.
func (p *Peer) fetchBlockFromPeer(ctx context.Context, pid peer.ID, c cid.Cid) (blockFetchReply, error) {
	s, err := p.host.NewStream(ctx, pid, blockFetchProtocol)
	if err != nil {
		return blockFetchReply{}, err
	}

	if err := cbor.NewEncoder(s).Encode(blockFetchRequest{CID: c.Bytes()}); err != nil {
		_ = s.Reset()
		return blockFetchReply{}, err
	}

	var reply blockFetchReply
	if err := cbor.NewDecoder(s).Decode(&reply); err != nil {
		_ = s.Reset()
		return blockFetchReply{}, err
	}

	_ = s.Close()
	return reply, nil
}

// remoteIPLDStore implements ipld-prime's storage.ReadableStorage and storage.WritableStorage.
//
// Reads check the local blockstore first. On a miss, it asks the peers in the
// fetch session (or, if none were supplied, every currently-connected peer)
// over the blockfetch protocol. The first peer that returns OK wins; the block
// is written through to the local blockstore so subsequent reads stay local.
//
// Writes go to the local blockstore only.
type remoteIPLDStore struct {
	peer *Peer
}

var (
	_ storage.ReadableStorage = (*remoteIPLDStore)(nil)
	_ storage.WritableStorage = (*remoteIPLDStore)(nil)
)

func (s *remoteIPLDStore) Has(ctx context.Context, key string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	c, err := cidFromKey(key)
	if err != nil {
		return false, err
	}
	return s.peer.blockstore.Has(ctx, c)
}

func (s *remoteIPLDStore) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c, err := cidFromKey(key)
	if err != nil {
		return nil, err
	}

	if has, herr := s.peer.blockstore.Has(ctx, c); herr == nil && has {
		block, gerr := s.peer.blockstore.Get(ctx, c)
		if gerr == nil {
			return block.RawData(), nil
		}
	}

	data, ok, err := s.peer.fetchRemote(ctx, c)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrBlockNotFound
	}

	block, err := blocks.NewBlockWithCid(data, c)
	if err != nil {
		return nil, err
	}
	if err := s.peer.blockstore.Put(ctx, block); err != nil {
		return nil, err
	}
	return data, nil
}

func (s *remoteIPLDStore) Put(ctx context.Context, key string, content []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c, err := cidFromKey(key)
	if err != nil {
		return err
	}
	block, err := blocks.NewBlockWithCid(content, c)
	if err != nil {
		return err
	}
	return s.peer.blockstore.Put(ctx, block)
}

// fetchRemote tries each candidate peer in turn and returns the first OK block.
//
// NotFound and Denied responses fall through to the next peer; transport
// errors do too. Returns (nil, false, nil) if no peer had the block.
func (p *Peer) fetchRemote(ctx context.Context, c cid.Cid) ([]byte, bool, error) {
	pids := p.candidatePeers(ctx)
	self := p.host.ID()

	for _, pid := range pids {
		if pid == self {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}

		peerCtx, cancel := context.WithTimeout(ctx, blockFetchPerPeerTimeout)
		reply, err := p.fetchBlockFromPeer(peerCtx, pid, c)
		cancel()
		if err != nil {
			continue
		}
		if reply.Status == blockFetchStatusOK {
			return reply.Data, true, nil
		}
	}
	return nil, false, nil
}

// candidatePeers returns the peer IDs that this fetch should try, in order.
//
// If the context carries an explicit session, those peers are used. Otherwise
// it falls back to every currently-connected peer (matching the broadcast-style
// behaviour of bitswap discovery flows).
func (p *Peer) candidatePeers(ctx context.Context) []peer.ID {
	sessIDs := sessionPeers(ctx)
	if len(sessIDs) > 0 {
		out := make([]peer.ID, 0, len(sessIDs))
		for _, s := range sessIDs {
			pid, err := peer.Decode(s)
			if err != nil {
				continue
			}
			out = append(out, pid)
		}
		return out
	}
	return p.host.Network().Peers()
}

func cidFromKey(key string) (cid.Cid, error) {
	n, c, err := cid.CidFromBytes([]byte(key))
	if err != nil {
		return cid.Undef, fmt.Errorf("blockfetch: key was not a cid: %w", err)
	}
	if n != len(key) {
		return cid.Undef, fmt.Errorf("blockfetch: key had %d trailing bytes", len(key)-n)
	}
	return c, nil
}
