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

// SizeOf is the Size of a sync that returned result and err. A failure other
// than a size refusal carries no counts, because the plan never came back.
func SizeOf(result *Result, err error) Size {
	size := Size{Ran: true, Failed: err != nil}

	var tooLarge *PlanTooLargeError

	switch {
	case errors.As(err, &tooLarge):
		size.Uploads, size.Downloads, size.Conflicts = tooLarge.Uploads, tooLarge.Downloads, tooLarge.Conflicts
		size.LocalDeletes, size.RemoteDeletes = tooLarge.LocalDeletes, tooLarge.RemoteDeletes
		size.OverRollback, size.OverRemoteDelete = tooLarge.OverRollback, tooLarge.OverRemoteDelete
	case err == nil && result != nil:
		size.Uploads, size.Downloads, size.Conflicts = result.UploadedCount, result.DownloadedCount, result.ConflictCount
		size.LocalDeletes, size.RemoteDeletes = result.LocalDeletedCount, result.RemoteDeletedCount
	}

	if err == nil && size == (Size{Ran: true}) {
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
