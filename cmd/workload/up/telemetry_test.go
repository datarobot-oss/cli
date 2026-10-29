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
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload/sync"
)

// executeAndTrack runs the command the way the root does, then builds the
// event the root's finalizer would send for it.
func executeAndTrack(t *testing.T, args ...string) (map[string]any, error) {
	t.Helper()

	cmd := Cmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs(args)
	cmd.PreRunE = nil

	err := cmd.Execute()

	event, ok := telemetry.EventFor(cmd, nil)
	require.True(t, ok)

	return event.EventProperties, err
}

// A sync refused for its size is the event the limit is sized from, so its
// counts travel even though the run failed.
func TestCmd_TelemetryReportsARefusedSyncSize(t *testing.T) {
	result := deployed()
	result.Sync = sync.Size{
		Ran: true, Failed: true, Uploads: 4, Downloads: 990, LocalDeletes: 7, Conflicts: 5, OverRollback: true,
	}

	stubRun(t, result, errors.New("plan would back up 1002 local files"))

	props, err := executeAndTrack(t, "--yes")
	require.Error(t, err)

	for key, want := range result.Sync.Properties() {
		assert.Equal(t, want, props[key], key)
	}

	assert.Equal(t, 1002, props["sync_rollback_files"])
	assert.Equal(t, true, props["yes"], "the flag properties are still there")
}

// The size a previous run on the same command left behind is never reported
// for a run that stopped before deploying.
func TestCmd_TelemetryForgetsAnEarlierRun(t *testing.T) {
	result := deployed()
	result.Sync = sync.Size{Ran: true, Uploads: 3}

	stubRun(t, result, nil)

	cmd := Cmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.PreRunE = nil

	cmd.SetArgs([]string{"--yes"})
	require.NoError(t, cmd.Execute())

	cmd.SetArgs([]string{"--yes", "--promote", "--detach"})
	require.Error(t, cmd.Execute(), "refused by its flags, before any deploy")

	event, ok := telemetry.EventFor(cmd, nil)
	require.True(t, ok)
	assert.Equal(t, false, event.EventProperties["sync_ran"])
}

// A run that synced nothing says so, rather than reporting zero counts that
// read like an empty sync.
func TestCmd_TelemetryReportsNoSync(t *testing.T) {
	stubRun(t, deployed(), nil)

	props, err := executeAndTrack(t, "--yes")
	require.NoError(t, err)

	assert.Equal(t, false, props["sync_ran"])
	assert.NotContains(t, props, "sync_uploads")
	assert.NotContains(t, props, "sync_limit_hit")
}
