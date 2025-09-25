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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sourcenetwork/corekv/memory"
)

func TestNewPeer_NoError(t *testing.T) {
	ctx := context.Background()
	store := memory.NewDatastore(ctx)
	p, err := NewPeer(ctx, WithRootstore(store))
	require.NoError(t, err)
	p.Close()
}

func TestStart_WithKnownPeer_NoError(t *testing.T) {
	ctx := context.Background()
	store := memory.NewDatastore(ctx)
	n1, err := NewPeer(
		ctx,
		WithRootstore(store),
		WithListenAddresses("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err)
	defer n1.Close()
	n2, err := NewPeer(
		ctx,
		WithRootstore(store),
		WithListenAddresses("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err)
	defer n2.Close()

	addrs, err := n1.Addresses()
	require.NoError(t, err)
	err = n2.Connect(ctx, addrs)
	require.NoError(t, err)
}

func TestNewPeer_WithEnableRelay_NoError(t *testing.T) {
	ctx := context.Background()
	store := memory.NewDatastore(ctx)
	n, err := NewPeer(
		context.Background(),
		WithRootstore(store),
		WithEnableRelay(true),
	)
	require.NoError(t, err)
	n.Close()
}

func TestNewPeer_NoPubSub_NoError(t *testing.T) {
	ctx := context.Background()
	store := memory.NewDatastore(ctx)
	n, err := NewPeer(
		context.Background(),
		WithRootstore(store),
		WithEnablePubSub(false),
	)
	require.NoError(t, err)
	require.Nil(t, n.ps)
	n.Close()
}

func TestNewPeer_WithEnablePubSub_NoError(t *testing.T) {
	ctx := context.Background()
	store := memory.NewDatastore(ctx)
	n, err := NewPeer(
		ctx,
		WithRootstore(store),
		WithEnablePubSub(true),
	)
	require.NoError(t, err)
	// overly simple check of validity of pubsub, avoiding the process of creating a PubSub
	require.NotNil(t, n.ps)
	n.Close()
}

func TestNodeClose_NoError(t *testing.T) {
	ctx := context.Background()
	store := memory.NewDatastore(ctx)
	n, err := NewPeer(
		context.Background(),
		WithRootstore(store),
	)
	require.NoError(t, err)
	n.Close()
}

func TestListenAddrs_WithListenAddresses_NoError(t *testing.T) {
	ctx := context.Background()
	store := memory.NewDatastore(ctx)
	n, err := NewPeer(
		context.Background(),
		WithRootstore(store),
		WithListenAddresses("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err)
	addrs, err := n.Addresses()
	require.NoError(t, err)
	require.NotEmpty(t, addrs)
	// ensure we have a tcp addr
	require.Contains(t, addrs[0], "/tcp/")
	n.Close()
}

func TestPeer_WithBootstrapPeers_NoError(t *testing.T) {
	ctx := context.Background()
	store := memory.NewDatastore(ctx)
	n, err := NewPeer(
		context.Background(),
		WithRootstore(store),
		WithBootstrapPeers("/ip4/127.0.0.1/tcp/6666/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ"),
	)
	require.NoError(t, err)

	n.Close()
}
