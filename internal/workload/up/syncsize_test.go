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

package up

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/datarobot/cli/internal/workload/sync"
)

func refusedPlan() error {
	return fmt.Errorf("phase execute: %w", &sync.PlanTooLargeError{
		Uploads: 40, Downloads: 900, LocalDeletes: 60, RemoteDeletes: 5, Conflicts: 41, OverRollback: true,
	})
}

var refusedSize = sync.Size{
	Ran: true, Failed: true, Uploads: 40, Downloads: 900, LocalDeletes: 60, RemoteDeletes: 5, Conflicts: 41,
	OverRollback: true,
}

// A first deploy refused for its size still reports the plan it was refused
// for: that run is the one the limit exists to be sized against.
func TestRun_FirstDeployReportsTheSyncSize(t *testing.T) {
	for name, tc := range map[string]struct {
		sync func(string) (*sync.Result, error)
		want sync.Size
	}{
		"refused": {
			sync: func(string) (*sync.Result, error) { return nil, refusedPlan() },
			want: refusedSize,
		},
		"failed for another reason": {
			sync: func(string) (*sync.Result, error) { return nil, errors.New("network down") },
			want: sync.Size{Ran: true, Failed: true},
		},
	} {
		t.Run(name, func(t *testing.T) {
			var tr track

			f := wiredBuild(&tr)
			f.sync = func(dir string) (*sync.Result, error) {
				tr.steps = append(tr.steps, "sync")

				return tc.sync(dir)
			}

			install(t, f)

			result, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
			require.Error(t, err)

			assert.Equal(t, []string{"create-artifact", "link", "sync"}, tr.steps)
			assert.Equal(t, tc.want, result.Sync)
		})
	}
}

// The roll path syncs inside the version it mints, so the size has to come
// back through that version, on a refusal as much as on a success.
func TestRun_RollReportsTheSyncSize(t *testing.T) {
	for name, tc := range map[string]struct {
		sync func(string) (*sync.Result, error)
		want sync.Size
	}{
		"applied": {
			sync: func(string) (*sync.Result, error) { return &sync.Result{UploadedCount: 2, NewVersion: "ver-2"}, nil },
			want: sync.Size{Ran: true, Uploads: 2},
		},
		"refused": {
			sync: func(string) (*sync.Result, error) { return nil, refusedPlan() },
			want: refusedSize,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var tr track

			f := builtRoll(&tr)
			f.linked = func(string) bool { return true }
			f.project = syncedProject("68a0000000000000000000a1")
			f.sync = tc.sync

			install(t, f)

			result, _, _ := runIn(t, builtDrift(), Options{NonInteractive: true})

			assert.Equal(t, tc.want, result.Sync)
		})
	}
}

// A run that never reaches the sync reports none.
func TestRun_NoSyncReportsNone(t *testing.T) {
	var tr track

	install(t, wiredBuild(&tr))

	result, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true, DryRun: true})
	require.NoError(t, err)

	assert.Equal(t, sync.Size{}, result.Sync)
}
