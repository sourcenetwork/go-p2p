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
	"time"

	"github.com/ipfs/boxo/blockservice"
	"github.com/ipld/go-ipld-prime/storage/bsrvadapter"
	libp2p "github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	dualdht "github.com/libp2p/go-libp2p-kad-dht/dual"
	record "github.com/libp2p/go-libp2p-record"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/sourcenetwork/corekv/blockstore"
	rpc "github.com/sourcenetwork/go-libp2p-pubsub-rpc"
	"github.com/sourcenetwork/immutable"
)

// setupHost returns a host and router configured with the given options.
func setupHost(ctx context.Context, options *Options) (host.Host, *dualdht.DHT, error) {
	connManager, err := connmgr.NewConnManager(100, 400, connmgr.WithGracePeriod(time.Second*20))
	if err != nil {
		return nil, nil, err
	}

	dhtOpts := []dualdht.Option{
		dualdht.DHTOption(dht.NamespacedValidator("pk", record.PublicKeyValidator{})),
		dualdht.DHTOption(dht.Concurrency(10)),
		dualdht.DHTOption(dht.Mode(dht.ModeAuto)),
	}

	var ddht *dualdht.DHT
	routing := func(h host.Host) (routing.PeerRouting, error) {
		ddht, err = dualdht.New(ctx, h, dhtOpts...)
		return ddht, err
	}

	libp2pOpts := []libp2p.Option{
		libp2p.ConnectionManager(connManager),
		libp2p.DefaultTransports,
		libp2p.ListenAddrStrings(options.ListenAddresses...),
		libp2p.Routing(routing),
	}

	// relay is enabled by default unless explicitly disabled
	if !options.EnableRelay {
		libp2pOpts = append(libp2pOpts, libp2p.DisableRelay())
	}

	// use the private key from options or generate a random one
	if options.PrivateKey != nil {
		privateKey, err := crypto.UnmarshalEd25519PrivateKey(options.PrivateKey)
		if err != nil {
			return nil, nil, err
		}
		libp2pOpts = append(libp2pOpts, libp2p.Identity(privateKey))
	}

	h, err := libp2p.New(libp2pOpts...)
	if err != nil {
		return nil, nil, err
	}
	return h, ddht, nil
}

func (p *Peer) ID() string {
	return p.host.ID().String()
}

// Addresses returns the full multiaddresses (with peer ID) of the host.
//
// If the host has no listen addresses, it will return just the /p2p/<PeerID> address.
func (p *Peer) Addresses() ([]string, error) {
	addrs := []string{}
	p2ppart, err := ma.NewComponent("p2p", p.host.ID().String())
	if err != nil {
		return nil, err
	}
	if len(p.host.Addrs()) == 0 {
		addrs = append(addrs, p2ppart.String())
		return addrs, nil
	}
	for _, addr := range p.host.Addrs() {
		addrs = append(addrs, addr.Encapsulate(p2ppart).String())
	}
	return addrs, nil
}

func (p *Peer) Pubkey() ([]byte, error) {
	return crypto.MarshalPublicKey(p.host.Peerstore().PubKey(p.host.ID()))
}

// ActivePeers returns the addresses of peers that are currently connected to.
//
// Addresses are returned in the multiaddr format (e.g. /ip4/127.0.0.1/tcp/4001/p2p/<PeerID>).
func (p *Peer) ActivePeers() ([]string, error) {
	addresses := []string{}

	for _, con := range p.host.Network().Conns() {
		pid := con.RemotePeer()
		address := con.RemoteMultiaddr()

		p2ppart, err := ma.NewComponent("p2p", pid.String())
		if err != nil {
			return nil, err
		}

		address = address.Encapsulate(p2ppart)

		addresses = append(addresses, address.String())
	}

	return addresses, nil
}

// Connect connects to a peer with the given addresses. If the list of addresses
// represents multiple peers, it will try to connect to all of them.
//
// Addresses should be in multiaddr format (e.g. /ip4/127.0.0.1/tcp/4001/p2p/<PeerID>).
func (p *Peer) Connect(ctx context.Context, addresses []string) error {
	addrs := []ma.Multiaddr{}
	for _, addr := range addresses {
		maddr, err := ma.NewMultiaddr(addr)
		if err != nil {
			return err
		}
		addrs = append(addrs, maddr)
	}
	addrInfos, err := peer.AddrInfosFromP2pAddrs(addrs...)
	if err != nil {
		return err
	}
	if len(addrInfos) == 0 {
		return errors.New("no valid addresses to connect to")
	}
	for _, addrInfo := range addrInfos {
		err = p.connect(ctx, addrInfo)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Peer) connect(ctx context.Context, addrInfo peer.AddrInfo) error {
	p.host.Peerstore().AddAddrs(addrInfo.ID, addrInfo.Addrs, peerstore.PermanentAddrTTL)
	return p.host.Connect(ctx, addrInfo)
}

func (p *Peer) Disconnect(ctx context.Context, peerID string) error {
	pid, err := peer.Decode(peerID)
	if err != nil {
		return err
	}
	p.host.Peerstore().ClearAddrs(pid)
	return nil
}

func (p *Peer) Send(ctx context.Context, data []byte, peerID string, protocolID string) error {
	pid, err := peer.Decode(peerID)
	if err != nil {
		return err
	}
	s, err := p.host.NewStream(ctx, pid, protocol.ID(protocolID))
	if err != nil {
		return err
	}
	defer func() {
		closeErr := s.Close()
		err = errors.Join(err, closeErr)
	}()

	_, err = s.Write(data)
	if err != nil {
		resetErr := s.Reset()
		return errors.Join(err, resetErr)
	}
	return s.Close()
}

func (p *Peer) Sign(data []byte) ([]byte, error) {
	return p.host.Peerstore().PrivKey(p.host.ID()).Sign(data)
}

func (p *Peer) SetStreamHandler(protocolID string, handler StreamHandler) {
	p.host.SetStreamHandler(protocol.ID(protocolID), func(stream network.Stream) {
		handler(stream, stream.Conn().RemotePeer().String())
	})
}

func (p *Peer) AddPubSubTopic(topicName string, subscribe bool, handler PubsubMessageHandler) error {
	messageHandler := func(from peer.ID, topic string, msg []byte) ([]byte, error) {
		return handler(from.String(), topic, msg)
	}
	_, err := p.addPubSubTopic(topicName, subscribe, messageHandler)
	return err
}

func (p *Peer) RemovePubSubTopic(topic string) error {
	return p.removePubSubTopic(topic)
}

// PublishToTopicAsync publishes the given data on the PubSub network via the
// corresponding topic asynchronously.
//
// This is a non blocking operation.
func (p *Peer) PublishToTopicAsync(ctx context.Context, topic string, data []byte) error {
	_, err := p.publishToTopic(ctx, topic, data, rpc.WithIgnoreResponse(true))
	return err
}

// PublishToTopic publishes the given data on the PubSub network via the
// corresponding topic.
//
// It will block until a response is received
func (p *Peer) PublishToTopic(
	ctx context.Context,
	topic string,
	data []byte,
	withMultiResponse bool,
) (<-chan PubsubResponse, error) {
	if withMultiResponse {
		return p.publishToTopic(ctx, topic, data, rpc.WithMultiResponse(true))
	}
	return p.publishToTopic(ctx, topic, data)
}

func (p *Peer) publishToTopic(
	ctx context.Context,
	topic string,
	data []byte,
	options ...rpc.PublishOption,
) (<-chan PubsubResponse, error) {
	if p.ps == nil { // skip if we aren't running with a pubsub net
		return nil, nil
	}

	p.topicMu.Lock()
	t, ok := p.topics[topic]
	p.topicMu.Unlock()
	if ok {
		resp, err := t.Publish(ctx, data, options...)
		if err != nil {
			return nil, NewErrPushLog(err, topic)
		}
		if resp != nil {
			respChan := make(chan PubsubResponse)
			go func() {
				for {
					select {
					case <-ctx.Done():
						close(respChan)
						return
					case r, ok := <-resp:
						if !ok {
							close(respChan)
							return
						}
						respChan <- PubsubResponse{
							ID:   r.ID,
							From: r.From.String(),
							Data: r.Data,
							Err:  r.Err,
						}
					}
				}
			}()
			return respChan, nil
		}
		return nil, nil
	}

	// If the topic hasn't been explicitly subscribed to, we temporarily join it
	// to publish the log.
	return nil, p.publishDirectToTopic(ctx, topic, data, false)
}

// IPLDStore returns the a wrapped blockservice.BlockService that implements the blockstore.IPLDStore interface.
func (p *Peer) IPLDStore() blockstore.IPLDStore {
	return &bsrvadapter.Adapter{Wrapped: p.blockService}
}

// ContextWithSession returns a context with a session for the blockservice.
func (p *Peer) ContextWithSession(ctx context.Context) context.Context {
	return blockservice.ContextWithSession(ctx, p.blockService)
}

func (p *Peer) SetBlockAccessFunc(accessFunc BlockAccessFunc) {
	p.accessFuncMu.Lock()
	defer p.accessFuncMu.Unlock()
	p.blockAccessFunc = immutable.Some(accessFunc)
}
