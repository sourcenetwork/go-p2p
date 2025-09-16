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
	"errors"
)

const (
	ErrPushLog           = "failed to push log"
	ErrTopicAlreadyExist = "topic already exists"
	ErrTopicDoesNotExist = "topic does not exists"
)

var (
	ErrTimeoutWaitingForPeerInfo = errors.New("timeout waiting for peer info")
	ErrContextDone               = errors.New("context done")
)

func NewErrPushLog(inner error, topic string) error {
	return errors.Join(inner, errors.New(ErrPushLog+". Topic: "+topic))
}

func NewErrTopicAlreadyExist(topic string) error {
	return errors.New(ErrTopicAlreadyExist + ". Topic: " + topic)
}

func NewErrTopicDoesNotExist(topic string) error {
	return errors.New(ErrTopicDoesNotExist + ". Topic: " + topic)
}
