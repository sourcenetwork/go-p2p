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
	"encoding/json"
	"io"

	"github.com/ipfs/go-cid"
)

type StreamHandler = func(stream io.Reader, peerID string)
type PubsubMessageHandler = func(from string, topic string, msg []byte) ([]byte, error)
type BlockAccessFunc = func(ctx context.Context, peerID string, c cid.Cid) bool

type PeerInfo struct {
	ID        string
	Addresses []string
}

func (p PeerInfo) String() string {
	b, _ := json.Marshal(p)
	return string(b)
}

type PubsubResponse struct {
	// ID is the cid.Cid of the received message.
	ID string
	// From is the ID of the sender.
	From string
	// Data is the message data.
	Data []byte
	// Err is an error from the sender.
	Err error
}
