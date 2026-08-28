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

package doctor

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func lockPath(t *testing.T, projectDir string) string {
	t.Helper()

	return filepath.Join(wapi.Dir(projectDir), sync.LockFileName)
}

func TestLockCheck_OK_AbsentFileNotCreated(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	res := (&lockCheck{projectDir: dir, goos: runtime.GOOS}).Run(context.Background())

	assert.Equal(t, core.StatusOK, res.Status)

	// The probe must be non-creating: a read-only diagnosis never leaves a
	// sync.lock behind.
	_, err := os.Stat(lockPath(t, dir))

	assert.ErrorIs(t, err, fs.ErrNotExist, "probe must not create sync.lock")
}

func TestLockCheck_OK_AcquirableAndReleasedWithinRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("flock semantics are unix-only; the windows path is covered by the seam test")
	}

	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, os.WriteFile(lockPath(t, dir), nil, 0o600))

	res := (&lockCheck{projectDir: dir, goos: runtime.GOOS}).Run(context.Background())

	assert.Equal(t, core.StatusOK, res.Status)

	// Prove the check released the lock before Run returned: a fresh open +
	// non-blocking exclusive lock in the SAME process must succeed now.
	f, err := os.OpenFile(lockPath(t, dir), os.O_RDWR, 0o600)

	require.NoError(t, err)

	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			t.Logf("close lock probe file: %v", closeErr)
		}
	}()

	require.NoError(t, tryLockSyncLockExclusive(f), "lock must be releasable within Run")

	require.NoError(t, unlockSyncLock(f))
}

func TestLockCheck_FAIL_HeldByLiveHolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("flock semantics are unix-only; the windows path is covered by the seam test")
	}

	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, os.WriteFile(lockPath(t, dir), nil, 0o600))

	// Hold the lock from a second open file description in this process —
	// flock contends between two fds of the same file, exactly like a live
	// second CLI process would.
	holder, err := os.OpenFile(lockPath(t, dir), os.O_RDWR, 0o600)

	require.NoError(t, err)

	require.NoError(t, tryLockSyncLockExclusive(holder))

	res := (&lockCheck{projectDir: dir, goos: runtime.GOOS}).Run(context.Background())

	assert.Equal(t, core.StatusFAIL, res.Status)

	assert.Contains(t, res.Summary, "held")

	assert.Equal(t, RemedyLockHeld, res.Remedy)

	// Release and confirm the check flips to OK.
	require.NoError(t, unlockSyncLock(holder))

	require.NoError(t, holder.Close())

	res = (&lockCheck{projectDir: dir, goos: runtime.GOOS}).Run(context.Background())

	assert.Equal(t, core.StatusOK, res.Status)
}

func TestLockCheck_WARN_CannotInspect(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission semantics; windows reports SKIP via the seam")
	}

	if os.Geteuid() == 0 {
		t.Skip("root can open unreadable files, so the WARN path cannot be fabricated")
	}

	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, os.WriteFile(lockPath(t, dir), nil, 0o600))

	require.NoError(t, os.Chmod(lockPath(t, dir), 0o000))

	t.Cleanup(func() {
		if chmodErr := os.Chmod(lockPath(t, dir), 0o600); chmodErr != nil {
			t.Logf("restore lock file permissions: %v", chmodErr)
		}
	})

	res := (&lockCheck{projectDir: dir, goos: runtime.GOOS}).Run(context.Background())

	assert.Equal(t, core.StatusWARN, res.Status)

	// An inspection failure must NEVER be misreported as a held lock.
	assert.Contains(t, res.Summary, "cannot inspect")

	assert.NotContains(t, res.Summary, "held")

	assert.Equal(t, RemedyLockInspect, res.Remedy)
}

func TestLockCheck_SKIP_WindowsSeam(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, os.WriteFile(lockPath(t, dir), nil, 0o600))

	// Even with a lock file present, the injected windows platform reports
	// SKIP and never reaches the flock probe.
	res := (&lockCheck{projectDir: dir, goos: "windows"}).Run(context.Background())

	assert.Equal(t, core.StatusSKIP, res.Status)

	assert.Contains(t, res.Summary, "lock not enforced on this platform")

	assert.Empty(t, res.Remedy)
}
