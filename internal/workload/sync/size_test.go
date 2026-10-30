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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSizeOf(t *testing.T) {
	cases := map[string]struct {
		result *Result
		err    error
		want   Size
	}{
		"applied": {
			result: &Result{
				UploadedCount: 3, DownloadedCount: 2, ConflictCount: 1,
				LocalDeletedCount: 4, RemoteDeletedCount: 5,
			},
			want: Size{Ran: true, Uploads: 3, Downloads: 2, Conflicts: 1, LocalDeletes: 4, RemoteDeletes: 5},
		},
		"refused by the rollback cap, through wrapping": {
			err: fmt.Errorf("cannot sync the project's code: %w", fmt.Errorf("phase execute: %w", &PlanTooLargeError{
				Uploads: 40, Downloads: 900, LocalDeletes: 60, RemoteDeletes: 5, Conflicts: 41, OverRollback: true,
			})),
			want: Size{
				Ran: true, Failed: true, Uploads: 40, Downloads: 900, LocalDeletes: 60, RemoteDeletes: 5, Conflicts: 41,
				OverRollback: true,
			},
		},
		"refused by both limits": {
			err: &PlanTooLargeError{Downloads: 1001, RemoteDeletes: 1001, OverRollback: true, OverRemoteDelete: true},
			want: Size{
				Ran: true, Failed: true, Downloads: 1001, RemoteDeletes: 1001, OverRollback: true, OverRemoteDelete: true,
			},
		},
		"failed partway through executing": {
			err: fmt.Errorf("cannot sync the project's code: %w", &ExecuteError{
				Size: Size{Uploads: 20000}, Err: errors.New("timeout waiting for archive extract"),
			}),
			want: Size{Ran: true, Failed: true, Uploads: 20000},
		},
		"applied, then failed to release the lock": {
			result: &Result{UploadedCount: 3},
			err:    &ExecuteError{Size: Size{Uploads: 3}, Err: errors.New("release lock: busy")},
			want:   Size{Ran: true, Failed: true, Uploads: 3},
		},
		"failed before there was a plan": {
			err:  errors.New("another sync holds the lock"),
			want: Size{},
		},
		// An up-to-date tree applies nothing, whether or not --verify still
		// executes its empty plan: both commands report no sync.
		"nothing to apply": {
			result: &Result{NewVersion: "v1"},
			want:   Size{},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, SizeOf(tc.result, tc.err))
		})
	}
}

func TestSize_Properties(t *testing.T) {
	assert.Equal(t, map[string]any{"sync_ran": false}, Size{}.Properties(),
		"a run that never synced reports only that, not zeros that read as an empty sync")

	assert.Equal(t, map[string]any{
		"sync_ran":                true,
		"sync_failed":             true,
		"sync_uploads":            40,
		"sync_downloads":          900,
		"sync_local_deletes":      60,
		"sync_remote_deletes":     5,
		"sync_conflicts":          41,
		"sync_rollback_files":     1001,
		"sync_limit_hit":          true,
		"sync_over_rollback":      true,
		"sync_over_remote_delete": false,
	}, Size{
		Ran: true, Failed: true, Uploads: 40, Downloads: 900, LocalDeletes: 60, RemoteDeletes: 5, Conflicts: 41,
		OverRollback: true,
	}.Properties())
}

// The applied result splits deletes the way the limits count them.
func TestPopulateResult_SplitsDeletes(t *testing.T) {
	e := &Engine{
		plan: &SyncPlan{Deletes: append(
			actions(2, ActDownloadDelete, "localdel"),
			actions(3, ActUploadDelete, "remotedel")...,
		)},
		nowFn: time.Now,
	}

	e.populateResult("v2")

	assert.Equal(t, 5, e.result.DeletedCount)
	assert.Equal(t, 2, e.result.LocalDeletedCount)
	assert.Equal(t, 3, e.result.RemoteDeletedCount)
}
