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
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncLock_AcquireRelease(t *testing.T) {
	dir := setupProject(t)

	lock, err := AcquireSyncLock(dir)
	require.NoError(t, err)
	require.NotNil(t, lock)

	require.NoError(t, lock.Release())
}

func TestSyncLock_DoubleAcquireFailsOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("v1 sync lock is a no-op on windows; tracked in RAPTOR-16928")
	}

	dir := setupProject(t)

	lock1, err := AcquireSyncLock(dir)
	require.NoError(t, err)

	t.Cleanup(func() { _ = lock1.Release() })

	_, err = AcquireSyncLock(dir)
	assert.Error(t, err, "second acquire on the same project must fail while the first is held")
}

func TestSyncLock_NoWapiDir(t *testing.T) {
	dir := t.TempDir() // no .wapi/

	_, err := AcquireSyncLock(dir)
	assert.Error(t, err)
}
