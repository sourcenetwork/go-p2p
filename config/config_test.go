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

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithListenAddresses(t *testing.T) {
	opts := &Options{}
	addresses := []string{"/ip4/127.0.0.1/tcp/6666", "/ip4/0.0.0.0/tcp/6666"}
	WithListenAddresses(addresses...)(opts)
	assert.Equal(t, addresses, opts.ListenAddresses)
}

func TestWithEnableRelay(t *testing.T) {
	opts := &Options{}
	WithEnableRelay(true)(opts)
	assert.Equal(t, true, opts.EnableRelay)
}

func TestWithEnablePubSub(t *testing.T) {
	opts := &Options{}
	WithEnablePubSub(true)(opts)
	assert.Equal(t, true, opts.EnablePubSub)
}

func TestWithBootstrapPeers(t *testing.T) {
	opts := &Options{}
	WithBootstrapPeers("/ip4/127.0.0.1/tcp/6666/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ")(opts)
	assert.ElementsMatch(t, []string{"/ip4/127.0.0.1/tcp/6666/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ"}, opts.BootstrapPeers)
}

func TestWithPrivateKey(t *testing.T) {
	opts := &Options{}
	WithPrivateKey([]byte("abc"))(opts)
	assert.Equal(t, []byte("abc"), opts.PrivateKey)
}
