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

import "errors"

// Size is what a sync applied, or the plan a size limit refused. Callers
// report it in telemetry, so the limits can be set from real projects rather
// than guessed. Counts only: it never carries a path.
type Size struct {
	// Ran says the run tried to apply changes. A zero Size is a run that
	// never got there or had nothing to apply, which both commands report
	// alike.
	Ran bool

	// Failed says the sync returned an error, a size refusal included.
	Failed bool

	Uploads       int
	Downloads     int
	LocalDeletes  int
	RemoteDeletes int
	Conflicts     int

	// The limits a refused plan went over.
	OverRollback     bool
	OverRemoteDelete bool
}

// ExecuteError is a failure once Execute had a plan, carrying the plan's
// counts so a sync that failed partway reports what it was applying.
type ExecuteError struct {
	Size Size
	Err  error
}

func (e *ExecuteError) Error() string { return e.Err.Error() }

func (e *ExecuteError) Unwrap() error { return e.Err }

// planSize counts what plan applies.
func planSize(plan *SyncPlan) Size {
	remoteDeletes := plan.RemoteDeletes()

	return Size{
		Uploads:       len(plan.Uploads),
		Downloads:     len(plan.Downloads),
		LocalDeletes:  len(plan.Deletes) - remoteDeletes,
		RemoteDeletes: remoteDeletes,
		Conflicts:     len(plan.Conflicts),
	}
}

// SizeOf is the Size of a sync that returned result and err. A failure before
// Execute had a plan is no sync at all.
func SizeOf(result *Result, err error) Size {
	var (
		tooLarge *PlanTooLargeError
		executed *ExecuteError
	)

	switch {
	case errors.As(err, &tooLarge):
		return Size{
			Ran: true, Failed: true,
			Uploads: tooLarge.Uploads, Downloads: tooLarge.Downloads, Conflicts: tooLarge.Conflicts,
			LocalDeletes: tooLarge.LocalDeletes, RemoteDeletes: tooLarge.RemoteDeletes,
			OverRollback: tooLarge.OverRollback, OverRemoteDelete: tooLarge.OverRemoteDelete,
		}
	case errors.As(err, &executed):
		size := executed.Size
		size.Ran, size.Failed = true, true

		return size
	case err != nil || result == nil:
		return Size{}
	}

	size := Size{
		Ran:     true,
		Uploads: result.UploadedCount, Downloads: result.DownloadedCount, Conflicts: result.ConflictCount,
		LocalDeletes: result.LocalDeletedCount, RemoteDeletes: result.RemoteDeletedCount,
	}

	if size == (Size{Ran: true}) {
		return Size{}
	}

	return size
}

// RollbackFiles is the count the rollback cap is compared against.
func (s Size) RollbackFiles() int {
	return s.Downloads + s.LocalDeletes + s.Conflicts
}

// Properties are the telemetry event properties for s. A run that applied
// nothing reports only that.
func (s Size) Properties() map[string]any {
	if !s.Ran {
		return map[string]any{"sync_ran": false}
	}

	return map[string]any{
		"sync_ran":                true,
		"sync_failed":             s.Failed,
		"sync_uploads":            s.Uploads,
		"sync_downloads":          s.Downloads,
		"sync_local_deletes":      s.LocalDeletes,
		"sync_remote_deletes":     s.RemoteDeletes,
		"sync_conflicts":          s.Conflicts,
		"sync_rollback_files":     s.RollbackFiles(),
		"sync_limit_hit":          s.OverRollback || s.OverRemoteDelete,
		"sync_over_rollback":      s.OverRollback,
		"sync_over_remote_delete": s.OverRemoteDelete,
	}
}
