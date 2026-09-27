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
	"fmt"
	"path"
	"slices"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/sourcenetwork/corelog"
	rpc "github.com/sourcenetwork/go-libp2p-pubsub-rpc"
)

// pubsubTopic is a wrapper of rpc.Topic to be able to track if the topic has
// been subscribed to.
type pubsubTopic struct {
	*rpc.Topic
	subscribed bool
}

// addPubSubTopic subscribes to a topic on the pubsub network
// A custom message handler can be provided to handle incoming messages. If not provided,
// the default message handler will be used.
func (p *Peer) addPubSubTopic(
	topic string,
	subscribe bool,
	handler rpc.MessageHandler,
	eventHandler rpc.EventHandler,
) (pubsubTopic, error) {
	if p.ps == nil {
		return pubsubTopic{}, nil
	}

	if handler == nil {
		return pubsubTopic{}, fmt.Errorf("handler cannot be nil")
	}

	log.InfoContext(p.ctx, "Adding pubsub topic",
		corelog.String("PeerID", p.host.ID().String()),
		corelog.String("Topic", topic))

	// Subscribing joins the topic, and so does a publish to a topic nobody
	// subscribes to. Whichever gets there second fails, so wait for the other
	// to be done. Locked before topicMu, the order publishing uses.
	unlock := p.lockTopicForDirectPublish(topic)
	defer unlock()

	p.topicMu.Lock()
	defer p.topicMu.Unlock()
	if t, ok := p.topics[topic]; ok {
		// When the topic was previously set to publish only and we now want to subscribe,
		// we need to close the existing topic and create a new one.
		if !t.subscribed && subscribe {
			if err := t.Close(); err != nil {
				return pubsubTopic{}, err
			}
		} else {
			return t, nil
		}
	}

	t, err := rpc.NewTopic(p.ctx, p.ps, p.host.ID(), topic, subscribe)
	if err != nil {
		return pubsubTopic{}, err
	}

	if eventHandler != nil {
		t.SetEventHandler(eventHandler)
	}
	t.SetMessageHandler(handler)
	pst := pubsubTopic{
		Topic:      t,
		subscribed: subscribe,
	}
	p.topics[topic] = pst
	return pst, nil
}

// removePubSubTopic unsubscribes to a topic
func (p *Peer) removePubSubTopic(topic string) error {
	if p.ps == nil {
		return nil
	}

	log.Info("Removing pubsub topic",
		corelog.String("PeerID", p.host.ID().String()),
		corelog.String("Topic", topic))

	p.topicMu.Lock()
	defer p.topicMu.Unlock()
	if t, ok := p.topics[topic]; ok {
		delete(p.topics, topic)
		return t.Close()
	}
	return nil
}

func (p *Peer) removeAllPubsubTopics() error {
	if p.ps == nil {
		return nil
	}

	log.Info("Removing all pubsub topics",
		corelog.String("PeerID", p.host.ID().String()))

	p.topicMu.Lock()
	defer p.topicMu.Unlock()
	for id, t := range p.topics {
		delete(p.topics, id)
		if err := t.Close(); err != nil {
			return err
		}
	}
	return nil
}

// topicLock is the queue for one topic. It is discarded once the last user
// hands it back, so a node publishing to many topics does not keep them all.
type topicLock struct {
	mu sync.Mutex
	// Holder plus whoever is waiting, guarded by topicLocksMu.
	refs int
}

// lockTopicForDirectPublish waits until nobody else holds the topic, and
// returns the function that hands it back.
func (p *Peer) lockTopicForDirectPublish(topic string) func() {
	p.topicLocksMu.Lock()
	l, ok := p.topicLocks[topic]
	if !ok {
		l = &topicLock{}
		p.topicLocks[topic] = l
	}
	// Counted before unlocking the map so the entry survives until we are done
	// with it, or a waiter could end up queueing on a lock nobody else holds.
	l.refs++
	p.topicLocksMu.Unlock()

	l.mu.Lock()
	return func() {
		l.mu.Unlock()

		p.topicLocksMu.Lock()
		l.refs--
		if l.refs == 0 {
			delete(p.topicLocks, topic)
		}
		p.topicLocksMu.Unlock()
	}
}

// publishDirectToTopic temporarily joins a pubsub topic to publish data and immediately closes it.
//
// This is useful to publish messages without incurring the cost of a full pubsub rpc topic.
//
// Returns false if the topic gained a subscriber in the meantime. Its handle
// stays open for good, so joining is no longer possible and the caller has to
// publish through it instead.
func (p *Peer) publishDirectToTopic(ctx context.Context, topic string, data []byte) (bool, error) {
	// The join below fails while anyone else holds this topic, so wait.
	unlock := p.lockTopicForDirectPublish(topic)
	defer unlock()

	// A subscriber may have taken the topic while we waited.
	p.topicMu.Lock()
	_, subscribed := p.topics[topic]
	p.topicMu.Unlock()
	if subscribed {
		return false, nil
	}

	psTopic, err := p.ps.Join(topic)
	if err != nil {
		return false, NewErrPushLog(err, topic)
	}
	err = psTopic.Publish(ctx, data)
	if err != nil {
		// Leaving it open would block every later publish to this topic.
		closeErr := psTopic.Close()
		return false, NewErrPushLog(errors.Join(err, closeErr), topic)
	}
	return true, psTopic.Close()
}

// TopicPeers returns the peers on the topic that this node can send to right
// now. Waiting for a peer to show up here before sending to it keeps a message
// sent right after connecting from being lost.
func (p *Peer) TopicPeers(topic string) []string {
	if p.ps == nil {
		return nil
	}
	peers := p.ps.ListPeers(topic)
	ids := make([]string, 0, len(peers))
	for _, id := range peers {
		ids = append(ids, id.String())
	}
	return ids
}

// replyRouteTimeout is longer than an asker waits for a reply by default, so a
// reply that has to wait longer would not be read anyway.
const replyRouteTimeout = 5 * time.Second

// waitForReplyRoute waits until this node can send a reply to the asker.
// Without it, a reply sent right after two nodes connect can be lost, and the
// asker times out.
func (p *Peer) waitForReplyRoute(topic string, asker peer.ID) {
	// Matches the name pubsub-rpc gives the reply topic.
	replyTopic := path.Join(topic, asker.String(), "_response")
	deadline := time.Now().Add(replyRouteTimeout)
	for time.Now().Before(deadline) {
		if slices.Contains(p.ps.ListPeers(replyTopic), asker) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
