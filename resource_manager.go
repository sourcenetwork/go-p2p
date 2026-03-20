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
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	iconnmgr "github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/network"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	connmgr "github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/pbnjay/memory"
)

const (
	connMgrGracePeriod      = 20 * time.Second
	defaultConnMgrLowWater  = 100
	defaultConnMgrHighWater = 400
)

var infiniteResourceLimits = rcmgr.InfiniteLimits.ToPartialLimitConfig().System

// connsFromMemory derives the maximum inbound connection count from a memory budget.
// Uses 1 connection per MB, matching Kubo's derivation.
func connsFromMemory(maxMem int64) int {
	return int(maxMem / (1024 * 1024))
}

// buildResourceManager constructs a libp2p resource manager derived from the given limits.
//
// Derivation rules adapted from Kubo (https://github.com/ipfs/kubo),
//   - System.Memory       = MaxMemory (or half of system RAM if zero)
//   - System.ConnsInbound = 1 per MB of MaxMemory
//   - Transient scope     = 25% of System scope
//   - PeerDefault         = autoscaled by libp2p (DefaultLimit sentinel)
//   - Service/Protocol    = unlimited
func buildResourceManager(limits ResourceLimits) (network.ResourceManager, error) {
	maxMem := limits.MaxMemory
	if maxMem == 0 {
		maxMem = int64(memory.TotalMemory()) / 2
	}
	fds := limits.MaxFileDescriptors
	conns := connsFromMemory(maxMem)

	partialLimits := rcmgr.PartialLimitConfig{
		System: rcmgr.ResourceLimits{
			Memory:       rcmgr.LimitVal64(maxMem),
			Conns:        rcmgr.Unlimited,
			ConnsInbound: rcmgr.LimitVal(conns),
		},
		// Transient connections are limited to 25% of system resources.
		Transient: rcmgr.ResourceLimits{
			Memory:       rcmgr.LimitVal64(maxMem / 4),
			Conns:        rcmgr.Unlimited,
			ConnsInbound: rcmgr.LimitVal(conns / 4),
		},
		// Constrain inbound only — guards against unintentional overuse by a single peer.
		// Not a DoS defense: an attacker can spin up multiple peer IDs.
		PeerDefault: rcmgr.ResourceLimits{
			ConnsInbound:   rcmgr.DefaultLimit,
			StreamsInbound: rcmgr.DefaultLimit,
		},
		// Keep service and protocol scopes unlimited for simplicity.
		ServiceDefault:      infiniteResourceLimits,
		ServicePeerDefault:  infiniteResourceLimits,
		ProtocolDefault:     infiniteResourceLimits,
		ProtocolPeerDefault: infiniteResourceLimits,
		Conn:                infiniteResourceLimits,
		Stream:              infiniteResourceLimits,
	}

	// Only apply FD limits if it's set, zero value will cause a block all limit
	if fds > 0 {
		partialLimits.System.FD = rcmgr.LimitVal(fds)
		partialLimits.Transient.FD = rcmgr.LimitVal(fds / 4)
	}

	scalingLimitConfig := rcmgr.DefaultLimits
	libp2p.SetDefaultServiceLimits(&scalingLimitConfig)
	limitConfig := partialLimits.Build(scalingLimitConfig.Scale(maxMem, fds))

	return rcmgr.NewResourceManager(rcmgr.NewFixedLimiter(limitConfig))
}

func defaultConnManager() (iconnmgr.ConnManager, error) {
	return connmgr.NewConnManager(defaultConnMgrLowWater, defaultConnMgrHighWater, connmgr.WithGracePeriod(connMgrGracePeriod))
}

func buildConnManager(limits ResourceLimits) (iconnmgr.ConnManager, error) {
	maxMem := limits.MaxMemory
	if maxMem == 0 {
		maxMem = int64(memory.TotalMemory()) / 2
	}
	// setting high watermark to half of the max connections
	highWater := connsFromMemory(maxMem) / 2
	// setting low watermark at 25% of high watermark
	lowWater := highWater / 4
	return connmgr.NewConnManager(lowWater, highWater, connmgr.WithGracePeriod(connMgrGracePeriod))
}

// buildResourceControls returns the connmgr and optional resource manager for the given options.
// The two are built together to ensure their limits are consistent with each other.
func buildResourceControls(options *Options) (iconnmgr.ConnManager, network.ResourceManager, error) {
	if options.ResourceManager != nil {
		cm, err := defaultConnManager()
		return cm, options.ResourceManager, err
	}

	if options.ResourceLimits.HasValue() {
		limits := options.ResourceLimits.Value()
		cm, err := buildConnManager(limits)
		if err != nil {
			return nil, nil, err
		}
		rm, err := buildResourceManager(limits)
		if err != nil {
			return nil, nil, err
		}
		return cm, rm, nil
	}

	// Neither set — preserve original defaults, no resource manager.
	cm, err := defaultConnManager()
	return cm, nil, err
}
