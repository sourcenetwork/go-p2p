# p2p

P2P package for SourceNetwork projects. Wraps a libp2p host with conveniences for
pubsub, peer management, and content-addressed block exchange.

## Block exchange (`/sourcenetwork/blockfetch/1.0.0`)

Block exchange is served by a small libp2p stream protocol — not bitswap.

### Why not bitswap

Bitswap is built for the open IPFS network: it maintains per-peer want-lists, runs
provider discovery through the DHT, scores peers, and broadcasts wants to every
connected peer. That comes with a fleet of background goroutines (session
manager, want-manager, peer manager, decision engine, response queue, …) and a
lot of cross-peer chatter.

Our usage pattern is much narrower:

- A sync is almost always triggered by a pubsub announcement from a known
  sender, so the peer that has the blocks is already in hand.
- Blocks are content-addressed and self-verifying, so we don't need bitswap's
  reputation / scoring machinery.
- DAG walks are sequential — one block at a time.

Bitswap's machinery is wasted under that pattern, and its scheduling patterns
misbehave on the single-threaded `GOOS=js GOARCH=wasm` runtime, where tight
coordination loops can starve cooperative goroutines.

### How it works

A directed request/response protocol over a libp2p stream:

1. Client opens a stream to the target peer on `/sourcenetwork/blockfetch/1.0.0`.
2. Client encodes one CBOR request: `{c: <CID bytes>}`.
3. Server reads the request, runs the access check, looks up the block locally,
   encodes one CBOR reply: `{s: status, d: <block bytes>}` where `status` is
   `0=OK`, `1=NotFound`, `2=Denied`.
4. Both sides close the stream.

One CID per stream, full-duplex, no message-id correlation, no background
workers. The stream handler goroutine is spawned by libp2p per inbound stream
and exits when the stream closes.

### Sessions and peer selection

`(*Peer).ContextWithSession(ctx, peerIDs...)` attaches a list of candidate peers
to the context. The IPLD store consults that list when serving a remote read:

- With explicit peer IDs, candidates are tried in order with a per-peer timeout.
  The first OK response wins; `NotFound` or `Denied` falls through to the next
  candidate. This is the targeted path — the caller already knows who has the
  data (typically the announcer of the pubsub message that triggered the sync).
- With no peer IDs, the store falls back to every currently-connected peer.
  This is the discovery path used by flows like collection-version sync where
  no specific sender is known up front.

### Access control

`(*Peer).SetBlockAccessFunc(fn)` installs a `BlockAccessFunc` predicate that the
server consults before serving each block. Returning `false` produces a `Denied`
reply. A nil func (the default) means open access, matching bitswap's
out-of-the-box behaviour.

### IPLD store

`(*Peer).IPLDStore()` returns a `storage.ReadableStorage` + `storage.WritableStorage`
implementation that:

- on read, checks the local blockstore first and falls back to remote peers via
  the protocol above, writing successful fetches through to the local store so
  subsequent reads stay local;
- on write, stores to the local blockstore only.

Plug this into a go-ipld-prime `LinkSystem` and DAG walks transparently fan out
to remote peers as needed.
