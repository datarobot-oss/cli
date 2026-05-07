// Copyright 2026 DataRobot, Inc. and its affiliates.
//
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

package sync

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureSpaceFor_Sufficient(t *testing.T) {
	t.Cleanup(restoreAvailableBytesFn(availableBytesFn))

	availableBytesFn = func(string) (int64, error) {
		return 10 * 1024 * 1024 * 1024, nil // 10 GiB free
	}

	require.NoError(t, EnsureSpaceFor("/anywhere", 1024*1024)) // 1 MiB needed
}

func TestEnsureSpaceFor_Insufficient(t *testing.T) {
	t.Cleanup(restoreAvailableBytesFn(availableBytesFn))

	availableBytesFn = func(string) (int64, error) {
		return 50 * 1024 * 1024, nil // 50 MiB free
	}

	const mib = 1024 * 1024

	err := EnsureSpaceFor("/anywhere", 100*mib) // need 100 + margin(100) = 200 MiB

	var ise *InsufficientSpaceError

	require.ErrorAs(t, err, &ise)
	assert.Equal(t, int64(50*mib), ise.FreeBytes)
}

func TestEnsureSpaceFor_StatfsError(t *testing.T) {
	t.Cleanup(restoreAvailableBytesFn(availableBytesFn))

	bad := errors.New("simulated statfs failure")
	availableBytesFn = func(string) (int64, error) { return 0, bad }

	err := EnsureSpaceFor("/anywhere", 1)
	require.ErrorIs(t, err, bad)
}

func restoreAvailableBytesFn(prev func(string) (int64, error)) func() {
	return func() { availableBytesFn = prev }
}
