package p2p

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/stretchr/testify/require"

	"github.com/sourcenetwork/corekv/memory"
)

// Probe: an open topic handle makes a second Join fail, which is the window
// publishDirectToTopic races against.
func TestProbe_JoinFailsWhileHandleOpen(t *testing.T) {
	ctx := context.Background()
	n, err := NewPeer(ctx, WithRootstore(memory.NewDatastore(ctx)), WithEnablePubSub(true))
	require.NoError(t, err)
	defer n.Close()

	h, err := n.ps.Join("probe-topic")
	require.NoError(t, err)

	_, err = n.ps.Join("probe-topic")
	require.Error(t, err)
	t.Logf("second join error: %v", err)

	require.NoError(t, h.Close())

	h2, err := n.ps.Join("probe-topic")
	require.NoError(t, err, "join should succeed once the handle is closed")
	require.NoError(t, h2.Close())
}

// Probe: concurrent publishDirectToTopic calls collide on the same topic.
func TestProbe_ConcurrentDirectPublishCollides(t *testing.T) {
	ctx := context.Background()
	n, err := NewPeer(ctx, WithRootstore(memory.NewDatastore(ctx)), WithEnablePubSub(true))
	require.NoError(t, err)
	defer n.Close()

	const workers = 16
	const rounds = 40
	var wg sync.WaitGroup
	errs := make(chan error, workers*rounds)
	for range workers {
		wg.Go(func() {
			for range rounds {
				if _, err := n.publishDirectToTopic(ctx, "hot-topic", []byte("x")); err != nil {
					errs <- err
				}
			}
		})
	}
	wg.Wait()
	close(errs)

	count := 0
	for e := range errs {
		count++
		if count == 1 {
			t.Logf("first failure: %v", e)
		}
	}
	t.Logf("publish failures after the single retry: %d / %d", count, workers*rounds)
}

// Probe: a subscription and a direct publish joining the same topic concurrently.
func TestProbe_SubscribeRacesDirectPublish(t *testing.T) {
	ctx := context.Background()
	n, err := NewPeer(ctx, WithRootstore(memory.NewDatastore(ctx)), WithEnablePubSub(true))
	require.NoError(t, err)
	defer n.Close()

	handler := func(from peer.ID, topic string, msg []byte) ([]byte, error) { return nil, nil }

	const rounds = 60
	var pubErrs, subErrs int
	var mu sync.Mutex
	for r := range rounds {
		topic := fmt.Sprintf("race-topic-%d", r)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := n.publishToTopic(ctx, topic, []byte("x")); err != nil {
				mu.Lock()
				pubErrs++
				mu.Unlock()
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := n.addPubSubTopic(topic, true, handler, nil); err != nil {
				mu.Lock()
				subErrs++
				if subErrs == 1 {
					t.Logf("first subscribe error: %v", err)
				}
				mu.Unlock()
			}
		}()
		wg.Wait()
	}
	t.Logf("publish errors: %d/%d, subscribe errors: %d/%d", pubErrs, rounds, subErrs, rounds)
}

// Probe: hammer subscribe, unsubscribe and publish on the same topics to shake
// out a lock-order deadlock. The timeout is the assertion.
func TestProbe_NoDeadlockUnderMixedLoad(t *testing.T) {
	ctx := context.Background()
	n, err := NewPeer(ctx, WithRootstore(memory.NewDatastore(ctx)), WithEnablePubSub(true))
	require.NoError(t, err)
	defer n.Close()

	handler := func(from peer.ID, topic string, msg []byte) ([]byte, error) { return nil, nil }

	done := make(chan struct{})
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		for w := range 12 {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				for r := range 50 {
					topic := fmt.Sprintf("mixed-%d", r%4)
					switch w % 3 {
					case 0:
						n.publishToTopic(ctx, topic, []byte("x"))
					case 1:
						n.addPubSubTopic(topic, true, handler, nil)
					case 2:
						n.removePubSubTopic(topic)
					}
				}
			}(w)
		}
		wg.Wait()
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("deadlock: mixed subscribe/unsubscribe/publish load did not finish")
	}
}
