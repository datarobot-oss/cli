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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/datarobot/cli/internal/workload/wapi"
)

// lockCheck probes the project's sync.lock NON-CREATINGLY: it opens the file
// WITHOUT O_CREATE and attempts a non-blocking exclusive advisory lock.
//
// It deliberately does NOT use sync.AcquireSyncLock, which creates the file
// (O_CREATE) and never removes it — using it for diagnosis would leave a new
// file behind and violate the read-only guarantee.
type lockCheck struct {
	projectDir string

	// goos is the injected platform seam: production constructs the check
	// with runtime.GOOS; tests inject "windows" to exercise the SKIP path
	// (flock is not enforced there, per RAPTOR-16928) on any host.
	goos string
}

// newLockCheck builds the lock check with the real host platform.
func newLockCheck(projectDir string) *lockCheck {
	return &lockCheck{projectDir: projectDir, goos: runtime.GOOS}
}

func (c *lockCheck) ID() string {
	return CheckIDLock
}

func (c *lockCheck) Name() string {
	return "Sync lock"
}

// Run classifies the lock file into four outcomes:
//   - absent (ENOENT)         -> OK, nothing held, and the file is NOT created
//   - open + acquire          -> OK, release immediately (release happens
//     inside Run, not at process exit)
//   - open + flock fails      -> FAIL, held by a live process
//   - open fails (perm / I/O) -> WARN "cannot inspect", never misreported
//     as "held by another process"
//
// On Windows (injected seam) it reports SKIP without touching the filesystem.
func (c *lockCheck) Run(_ context.Context) core.Result {
	if c.goos == "windows" {
		return core.Result{
			Status:  core.StatusSKIP,
			Summary: "lock not enforced on this platform",
		}
	}

	if res, skip := skipIfUnlinked(c.projectDir); skip {
		return res
	}

	path := filepath.Join(wapi.Dir(c.projectDir), sync.LockFileName)

	// No O_CREATE: a read-only diagnosis must never leave a lock file behind.
	f, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return core.Result{
				Status:  core.StatusOK,
				Summary: "no sync lock file; nothing held",
			}
		}

		return core.Result{
			Status:  core.StatusWARN,
			Summary: fmt.Sprintf("cannot inspect sync lock: %s", err),
			Remedy:  RemedyLockInspect,
		}
	}

	if lockErr := tryLockSyncLockExclusive(f); lockErr != nil {
		// Release nothing: we never owned the lock. Close errors here are
		// non-actionable (read-only descriptor teardown).
		_ = f.Close()

		return core.Result{
			Status:  core.StatusFAIL,
			Summary: "sync lock held by a live process",
			Remedy:  RemedyLockHeld,
		}
	}

	if releaseErr := releaseSyncLock(f); releaseErr != nil {
		return core.Result{
			Status:  core.StatusWARN,
			Summary: fmt.Sprintf("cannot inspect sync lock: %s", releaseErr),
			Remedy:  RemedyLockInspect,
		}
	}

	return core.Result{
		Status:  core.StatusOK,
		Summary: "sync lock is acquirable (no live holder)",
	}
}

// releaseSyncLock unlocks and closes the probe's descriptor. It must only be
// called after a successful acquire.
func releaseSyncLock(f *os.File) error {
	if err := unlockSyncLock(f); err != nil {
		return fmt.Errorf("unlock sync lock: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("close sync lock: %w", err)
	}

	return nil
}
