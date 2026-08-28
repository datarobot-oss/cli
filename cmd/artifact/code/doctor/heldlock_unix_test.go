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

//go:build !windows

package doctor

import (
	"os"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/stretchr/testify/require"
)

// holdSyncLock acquires the same exclusive advisory lock the sync engine
// uses, from this test process, to simulate a live second CLI process
// holding the sync lock. The returned function releases it.
func holdSyncLock(t *testing.T, path string) func() {
	t.Helper()

	f, err := os.OpenFile(path, os.O_RDWR, 0o600)

	require.NoError(t, err)

	require.NoError(t, unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)) //nolint:gosec // uintptr and int are same size on supported platforms

	return func() {
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN) //nolint:gosec // uintptr and int are same size on supported platforms

		_ = f.Close()
	}
}
