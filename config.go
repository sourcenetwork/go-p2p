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

/* Node configuration, in which NodeOpt functions are applied on Options. */

package p2p

import (
	"github.com/sourcenetwork/corekv"
	"github.com/sourcenetwork/immutable"
)

// Options is the node options.
type Options struct {
	ListenAddresses []string
	PrivateKey      []byte
	EnablePubSub    bool
	EnableRelay     bool
	BootstrapPeers  []string

	Blockstore          immutable.Option[Blockstore]
	Rootstore           immutable.Option[corekv.ReaderWriter]
	BlockstoreNamespace string
}

// DefaultOptions returns the default net options.
func DefaultOptions() *Options {
	return &Options{
		ListenAddresses:     []string{"/ip4/0.0.0.0/tcp/9171"},
		EnablePubSub:        true,
		EnableRelay:         false,
		BlockstoreNamespace: "/blocks",
	}
}

type NodeOpt func(*Options)

// WithPrivateKey sets the p2p host private key.
func WithPrivateKey(priv []byte) NodeOpt {
	return func(opt *Options) {
		opt.PrivateKey = priv
	}
}

// WithEnablePubSub enables the pubsub feature.
func WithEnablePubSub(enable bool) NodeOpt {
	return func(opt *Options) {
		opt.EnablePubSub = enable
	}
}

// WithEnableRelay enables the relay feature.
func WithEnableRelay(enable bool) NodeOpt {
	return func(opt *Options) {
		opt.EnableRelay = enable
	}
}

// WithListenAddress sets the address to listen on given as a multiaddress string.
func WithListenAddresses(addresses ...string) NodeOpt {
	return func(opt *Options) {
		opt.ListenAddresses = addresses
	}
}

// WithBootstrapPeers sets the bootstrap peer addresses to attempt to connect to.
func WithBootstrapPeers(peers ...string) NodeOpt {
	return func(opt *Options) {
		opt.BootstrapPeers = peers
	}
}

// WithBootstrapPeers sets the backing blockstore that the Peer will use to send/receive/store blocks.
//
// Providing either Blockstore or Rootstore is required.
func WithBlockstore(blockstore Blockstore) NodeOpt {
	return func(opt *Options) {
		opt.Blockstore = immutable.Some(blockstore)
	}
}

// WithRootstore sets the backing store that the Peer will use to send/receive/store blocks.
//
// Providing either Blockstore or Rootstore is required.
func WithRootstore(root corekv.ReaderWriter) NodeOpt {
	return func(opt *Options) {
		opt.Rootstore = immutable.Some(root)
	}
}

// WithBlockstoreNamespace sets the namespace within the provided Rootstore that the Peer
// will use to send/receive/store blocks.
//
// If Rootstore is not provided this option has no impact.
func WithBlockstoreNamespace(path string) NodeOpt {
	return func(opt *Options) {
		opt.BlockstoreNamespace = path
	}
}
