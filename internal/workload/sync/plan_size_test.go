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
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
)

func actions(n int, action Action, prefix string) []FileAction {
	out := make([]FileAction, n)

	for i := range out {
		out[i] = FileAction{Path: fmt.Sprintf("%s%05d.txt", prefix, i), Action: action}
	}

	return out
}

func TestSyncPlan_RollbackFilesAndRemoteDeletes(t *testing.T) {
	plan := &SyncPlan{
		Uploads:   actions(7, ActUploadAdd, "up"),
		Downloads: actions(3, ActDownloadModify, "down"),
		Deletes: append(
			actions(2, ActDownloadDelete, "localdel"),
			actions(5, ActUploadDelete, "remotedel")...,
		),
		Conflicts: actions(1, ActConflictCopy, "conflict"),
	}

	assert.Equal(t, 6, plan.RollbackFiles(), "downloads, local deletes and conflicts; never uploads")
	assert.Equal(t, 5, plan.RemoteDeletes())
}

func TestCheckPlanSize(t *testing.T) {
	cases := map[string]struct {
		plan    *SyncPlan
		wantErr string
	}{
		"a first deploy of a large project uploads freely": {
			plan: &SyncPlan{Uploads: actions(5*RollbackMaxFiles, ActUploadAdd, "up")},
		},
		"backups exactly at the cap": {
			plan: &SyncPlan{
				Downloads: actions(RollbackMaxFiles-2, ActDownloadModify, "down"),
				Deletes:   actions(1, ActDownloadDelete, "localdel"),
				Conflicts: actions(1, ActConflictCopy, "conflict"),
			},
		},
		"one backup over the cap": {
			plan: &SyncPlan{
				Downloads: actions(RollbackMaxFiles-1, ActDownloadModify, "down"),
				Deletes:   actions(1, ActDownloadDelete, "localdel"),
				Conflicts: actions(1, ActConflictCopy, "conflict"),
			},
			wantErr: fmt.Sprintf("plan would back up %d local files (downloads=%d, local deletes=1, conflicts=1), exceeding RollbackMaxFiles=%d",
				RollbackMaxFiles+1, RollbackMaxFiles-1, RollbackMaxFiles),
		},
		"remote deletes do not count as backups": {
			plan: &SyncPlan{
				Downloads: actions(RollbackMaxFiles, ActDownloadModify, "down"),
				Deletes:   actions(RemoteDeleteMaxPaths, ActUploadDelete, "remotedel"),
			},
		},
		"more remote deletes than one request may name": {
			plan:    &SyncPlan{Deletes: actions(RemoteDeleteMaxPaths+1, ActUploadDelete, "remotedel")},
			wantErr: fmt.Sprintf("plan deletes %d remote files, more than the %d the Files API accepts", RemoteDeleteMaxPaths+1, RemoteDeleteMaxPaths),
		},
		"both limits are named": {
			plan: &SyncPlan{
				Downloads: actions(RollbackMaxFiles+1, ActDownloadModify, "down"),
				Deletes:   actions(RemoteDeleteMaxPaths+1, ActUploadDelete, "remotedel"),
			},
			wantErr: fmt.Sprintf("exceeding RollbackMaxFiles=%d; plan deletes %d remote files", RollbackMaxFiles, RemoteDeleteMaxPaths+1),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := checkPlanSize(tc.plan)

			if tc.wantErr == "" {
				assert.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// A large archive gets longer than the fixed wait to extract, a small one
// never less, and none more than the cap.
func TestZipPollTimeout(t *testing.T) {
	floor := time.Duration(ZipPollTimeoutSecs) * time.Second
	ceiling := time.Duration(ZipPollMaxTimeoutSecs) * time.Second

	assert.Equal(t, floor, zipPollTimeout(1))
	assert.Equal(t, floor, zipPollTimeout(int(floor/ZipPollPerFile)))
	assert.Equal(t, 20000*ZipPollPerFile, zipPollTimeout(20000))
	assert.Equal(t, ceiling, zipPollTimeout(int(ceiling/ZipPollPerFile)))
	assert.Equal(t, ceiling, zipPollTimeout(100000))
}

// The refusal survives the phase wrapping as a typed error with the counts,
// which is what telemetry reads.
func TestCheckPlanSize_ReportsCounts(t *testing.T) {
	plan := &SyncPlan{
		Uploads:   actions(4, ActUploadAdd, "up"),
		Downloads: actions(RollbackMaxFiles, ActDownloadModify, "down"),
		Deletes: append(
			actions(2, ActDownloadDelete, "localdel"),
			actions(3, ActUploadDelete, "remotedel")...,
		),
	}

	err := runPhases(&Engine{plan: plan}, phase{name: "execute", run: func(e *Engine) error { return checkPlanSize(e.plan) }})

	var tooLarge *PlanTooLargeError

	require.ErrorAs(t, err, &tooLarge)
	assert.Equal(t, PlanTooLargeError{
		Uploads: 4, Downloads: RollbackMaxFiles, LocalDeletes: 2, RemoteDeletes: 3, OverRollback: true,
	}, *tooLarge)

	err = checkPlanSize(&SyncPlan{Deletes: actions(RemoteDeleteMaxPaths+1, ActUploadDelete, "remotedel")})

	require.ErrorAs(t, err, &tooLarge)
	assert.False(t, tooLarge.OverRollback)
	assert.True(t, tooLarge.OverRemoteDelete)
	assert.Equal(t, RemoteDeleteMaxPaths+1, tooLarge.RemoteDeletes)
}

// A failure inside Execute carries the plan's counts, so telemetry can report
// what a sync that failed partway was applying.
func TestExecute_FailureCarriesThePlanSize(t *testing.T) {
	e := &Engine{
		plan: &SyncPlan{
			Uploads: actions(2, ActUploadAdd, "up"),
			Deletes: append(actions(1, ActDownloadDelete, "localdel"), actions(3, ActUploadDelete, "remotedel")...),
		},
		artifact: &workload.Artifact{ID: "art-1", Status: workload.ArtifactStatusLocked},
	}

	_, err := e.Execute(e.plan)

	var executed *ExecuteError

	require.ErrorAs(t, err, &executed)
	assert.Contains(t, err.Error(), "is locked")
	assert.Equal(t, Size{Uploads: 2, LocalDeletes: 1, RemoteDeletes: 3}, executed.Size)
}

// The refusal comes before anything touches the tree: no rollback dir.
func TestPhase5Execute_RefusesOversizedPlanBeforeRollback(t *testing.T) {
	dir := t.TempDir()

	e := &Engine{
		projectDir: dir,
		plan:       &SyncPlan{Downloads: actions(RollbackMaxFiles+1, ActDownloadModify, "down")},
	}

	err := phase5Execute(e)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeding RollbackMaxFiles")

	_, statErr := os.Stat(wapi.RollbackDir(dir))
	assert.True(t, os.IsNotExist(statErr), "no rollback dir for a refused plan")
}
