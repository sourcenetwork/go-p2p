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
	"fmt"
	"testing"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/sourcenetwork/immutable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testMemory128MB = 128 * 1024 * 1024

// TestConnsFromMemory verifies the 1-conn-per-MB derivation rule.
func TestConnsFromMemory(t *testing.T) {
	assert.Equal(t, 128, connsFromMemory(128*1024*1024))
	assert.Equal(t, 1024, connsFromMemory(1024*1024*1024))
}

// TestBuildResourceManager_SystemConnsInbound verifies that system-scope inbound
// connections are capped at 1 per MB of MaxMemory.
func TestBuildResourceManager_SystemConnsInbound(t *testing.T) {
	rm, err := buildResourceManager(ResourceLimits{MaxMemory: testMemory128MB})
	require.NoError(t, err)
	defer rm.Close()

	conns := make([]network.ConnManagementScope, 128)
	defer func() {
		for _, c := range conns {
			if c != nil {
				c.Done()
			}
		}
	}()

	for i := range conns {
		addr := multiaddr.StringCast(fmt.Sprintf("/ip4/1.2.3.%d/tcp/1234", i%255+1))
		conns[i], err = rm.OpenConnection(network.DirInbound, false, addr)
		require.NoError(t, err)
		// Move out of transient scope so the system limit is exercised.
		err = conns[i].SetPeer(peer.ID(fmt.Sprintf("peer%d", i)))
		require.NoError(t, err)
	}

	addr := multiaddr.StringCast("/ip4/1.3.4.1/tcp/1234")
	_, err = rm.OpenConnection(network.DirInbound, false, addr)
	assert.Error(t, err, "should reject 129th system inbound connection")
}

// TestBuildResourceManager_TransientConnsInbound verifies that transient inbound
// connections are capped at 25% of the system inbound limit.
func TestBuildResourceManager_TransientConnsInbound(t *testing.T) {
	rm, err := buildResourceManager(ResourceLimits{MaxMemory: testMemory128MB})
	require.NoError(t, err)
	defer rm.Close()

	conns := make([]network.ConnManagementScope, 32)
	defer func() {
		for _, c := range conns {
			if c != nil {
				c.Done()
			}
		}
	}()

	for i := range conns {
		addr := multiaddr.StringCast(fmt.Sprintf("/ip4/1.2.3.%d/tcp/1234", i%255+1))
		conns[i], err = rm.OpenConnection(network.DirInbound, false, addr)
		require.NoError(t, err)
	}

	addr := multiaddr.StringCast("/ip4/1.3.4.1/tcp/1234")
	_, err = rm.OpenConnection(network.DirInbound, false, addr)
	assert.Error(t, err, "should reject 33rd transient inbound connection")
}

// TestBuildResourceManager_ZeroMemory verifies that a zero MaxMemory falls back
// to half of system RAM without error.
func TestBuildResourceManager_ZeroMemory(t *testing.T) {
	rm, err := buildResourceManager(ResourceLimits{})
	require.NoError(t, err)
	assert.NotNil(t, rm)
	rm.Close()
}

// TestBuildResourceControls_NegativeMemory verifies that negative MaxMemory is rejected.
func TestBuildResourceControls_NegativeMemory(t *testing.T) {
	opts := &Options{
		ResourceLimits: immutable.Some(ResourceLimits{MaxMemory: -1}),
	}
	_, _, err := buildResourceControls(opts)
	assert.ErrorIs(t, err, ErrNegativeMaxMemory)
}

// TestBuildResourceControls_NegativeFDs verifies that negative MaxFileDescriptors is rejected.
func TestBuildResourceControls_NegativeFDs(t *testing.T) {
	opts := &Options{
		ResourceLimits: immutable.Some(ResourceLimits{MaxFileDescriptors: -1}),
	}
	_, _, err := buildResourceControls(opts)
	assert.ErrorIs(t, err, ErrNegativeMaxFileDescriptors)
}

// TestBuildResourceControls_MemoryTooLow verifies that a non-zero MaxMemory below 128 MiB is rejected.
func TestBuildResourceControls_MemoryTooLow(t *testing.T) {
	opts := &Options{
		ResourceLimits: immutable.Some(ResourceLimits{MaxMemory: 64 << 20}),
	}
	_, _, err := buildResourceControls(opts)
	assert.ErrorIs(t, err, ErrMaxMemoryTooLow)
}

// TestBuildResourceControls_FDsTooLow verifies that a non-zero MaxFileDescriptors below 128 is rejected.
func TestBuildResourceControls_FDsTooLow(t *testing.T) {
	opts := &Options{
		ResourceLimits: immutable.Some(ResourceLimits{MaxMemory: testMemory128MB, MaxFileDescriptors: 128}),
	}
	_, _, err := buildResourceControls(opts)
	assert.ErrorIs(t, err, ErrMaxFileDescriptorsTooLow)
}

// TestBuildResourceControls_NeitherSet verifies default connmgr is returned
// with no resource manager when neither option is set.
func TestBuildResourceControls_NeitherSet(t *testing.T) {
	cm, rm, err := buildResourceControls(&Options{})
	require.NoError(t, err)
	assert.NotNil(t, cm)
	assert.Nil(t, rm)
}

// TestBuildResourceControls_LimitsOnly verifies that both connmgr and resource
// manager are returned when only ResourceLimits is set.
func TestBuildResourceControls_LimitsOnly(t *testing.T) {
	opts := &Options{
		ResourceLimits: immutable.Some(ResourceLimits{MaxMemory: testMemory128MB}),
	}
	cm, rm, err := buildResourceControls(opts)
	require.NoError(t, err)
	assert.NotNil(t, cm)
	assert.NotNil(t, rm)
	rm.Close()
}

// TestBuildResourceControls_ResourceManagerOverrides verifies that when ResourceManager
// is set it takes precedence over ResourceLimits.
func TestBuildResourceControls_ResourceManagerOverrides(t *testing.T) {
	customRM := &network.NullResourceManager{}
	opts := &Options{
		ResourceManager: customRM,
		ResourceLimits:  immutable.Some(ResourceLimits{MaxMemory: testMemory128MB}),
	}
	cm, rm, err := buildResourceControls(opts)
	require.NoError(t, err)
	assert.NotNil(t, cm)
	assert.Equal(t, customRM, rm)

}
