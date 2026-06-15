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

// phase0Preflight does stale-rollback recovery, .wapi/ presence check,
// and acquires the project lock. Recovery runs before the lock so a
// crashed-mid-sync process gets cleaned up by whoever runs next.
func phase0Preflight(e *Engine) error {
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
