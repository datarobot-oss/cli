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
	"fmt"

	"github.com/datarobot/cli/internal/workload/wapi"
)

// phase0Preflight migrates legacy state, does stale-rollback recovery, checks
// the project is linked, and acquires the project lock. Migration runs first
// so everything after it resolves one state directory; recovery runs before
// the lock so a crashed-mid-sync process gets cleaned up by whoever runs next.
//
// The preview modes skip the migration: --dry-run and --diff should not move
// anything on disk, and they read fine either way because the path helpers
// fall back to the legacy location on their own.
func phase0Preflight(e *Engine) error {
	if !e.opts.DryRun && !e.opts.ShowDiffs {
		wapi.EnsureMigrated(e.projectDir)
	}

	restored, err := RestoreStaleIfPresent(e.projectDir)
	if err != nil {
		return fmt.Errorf("recover stale rollback: %w", err)
	}

	e.staleNote = restored

	if !wapi.Exists(e.projectDir) {
		return errors.New("not linked: run 'dr artifact code init <artifact-id>' first")
	}

	lock, err := AcquireSyncLock(e.projectDir)
	if err != nil {
		return err
	}

	e.lock = lock

	return nil
}
