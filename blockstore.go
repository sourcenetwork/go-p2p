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

	"github.com/ipfs/boxo/blockstore"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/storage"
)

// Blockstore proxies the ipld.DAGService under the /core namespace for future-proofing
type Blockstore interface {
	blockstore.Blockstore
	AsIPLDStorage() IPLDStorage
	// Mark the block as merged by removing the to-merge index.
	MarkAsMerged(ctx context.Context, k cid.Cid) error
	// Check if the block has been merged. It will return false if either the CID is not found
	// or the CID is found AND the to-mege index is aslo found.
	IsMerged(ctx context.Context, k cid.Cid) (bool, error)
}

// IPLDStorage provides the methods needed for an IPLD LinkSystem.
type IPLDStorage interface {
	storage.ReadableStorage
	storage.WritableStorage
}
