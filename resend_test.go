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
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sourcenetwork/corekv/memory"
)

// A request published before the other peer subscribes still gets its reply
// once that peer does.
func TestPublishToTopic_PeerSubscribesAfterRequest_GetsReply(t *testing.T) {
	ctx := context.Background()
	newPeer := func() *Peer {
		p, err := NewPeer(
			ctx,
			WithRootstore(memory.NewDatastore(ctx)),
			WithListenAddresses("/ip4/127.0.0.1/tcp/0"),
			WithEnablePubSub(true),
		)
		require.NoError(t, err)
		return p
	}
	requester := newPeer()
	defer requester.Close()
	responder := newPeer()
	defer responder.Close()

	addrs, err := responder.Addresses()
	require.NoError(t, err)
	require.NoError(t, requester.Connect(ctx, addrs))

	const topic = "late-subscriber"
	noop := func(from, topic string, msg []byte) ([]byte, error) { return nil, nil }
	require.NoError(t, requester.AddPubSubTopic(topic, true, noop, nil))

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	replies, err := requester.PublishToTopic(reqCtx, topic, []byte("request"), true)
	require.NoError(t, err)

	// Subscribes only now, so the request above had nowhere to go when it was sent.
	reply := func(from, topic string, msg []byte) ([]byte, error) { return []byte("reply"), nil }
	require.NoError(t, responder.AddPubSubTopic(topic, true, reply, nil))

	select {
	case resp, ok := <-replies:
		require.True(t, ok, "reply channel closed without a reply")
		require.NoError(t, resp.Err)
		require.Equal(t, "reply", string(resp.Data))
	case <-reqCtx.Done():
		t.Fatal("no reply: the request was not resent once the peer subscribed")
	}
}

func TestTopicPeers_ListsSubscribedPeer(t *testing.T) {
	ctx := context.Background()
	newPeer := func() *Peer {
		p, err := NewPeer(
			ctx,
			WithRootstore(memory.NewDatastore(ctx)),
			WithListenAddresses("/ip4/127.0.0.1/tcp/0"),
			WithEnablePubSub(true),
		)
		require.NoError(t, err)
		return p
	}
	a := newPeer()
	defer a.Close()
	b := newPeer()
	defer b.Close()

	addrs, err := b.Addresses()
	require.NoError(t, err)
	require.NoError(t, a.Connect(ctx, addrs))

	const topic = "topic-peers"
	noop := func(from, topic string, msg []byte) ([]byte, error) { return nil, nil }
	require.NoError(t, a.AddPubSubTopic(topic, true, noop, nil))
	require.Empty(t, a.TopicPeers(topic), "b has not subscribed yet")

	require.NoError(t, b.AddPubSubTopic(topic, true, noop, nil))
	require.Eventually(t, func() bool {
		peers := a.TopicPeers(topic)
		return len(peers) == 1 && peers[0] == b.ID()
	}, 5*time.Second, 10*time.Millisecond)
}
