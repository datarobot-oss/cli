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

package codesync

import (
	"errors"
	"testing"

	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The size of an executed sync rides on the event, refused or not: pulls are
// what the rollback cap still limits, and this command is where they happen.
func TestTelemetry_ReportsTheSyncSize(t *testing.T) {
	plan := &sync.SyncPlan{Uploads: []sync.FileAction{{Path: "a.py"}}}

	for name, tc := range map[string]struct {
		engine *fakeEngine
		want   sync.Size
	}{
		"applied": {
			engine: &fakeEngine{plan: plan, result: &sync.Result{NewVersion: "v2", UploadedCount: 1, DownloadedCount: 4}},
			want:   sync.Size{Ran: true, Uploads: 1, Downloads: 4},
		},
		"refused for its size": {
			engine: &fakeEngine{plan: plan, executeErr: &sync.PlanTooLargeError{Downloads: 1001, OverRollback: true}},
			want:   sync.Size{Ran: true, Failed: true, Downloads: 1001, OverRollback: true},
		},
		"failed for another reason": {
			engine: &fakeEngine{plan: plan, executeErr: errors.New("upload failed mid-sync")},
			want:   sync.Size{Ran: true, Failed: true},
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			linkProject(t, dir)

			cmd, _, _, _ := runWithDeps(t, fakeEngineDeps(tc.engine), map[string]string{"dir": dir, "yes": "true"})

			event, ok := telemetry.EventFor(cmd, nil)
			require.True(t, ok)

			for key, want := range tc.want.Properties() {
				assert.Equal(t, want, event.EventProperties[key], key)
			}
		})
	}
}

// A preview executes nothing, so it reports no sync.
func TestTelemetry_PreviewReportsNoSync(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{plan: &sync.SyncPlan{Uploads: []sync.FileAction{{Path: "a.py"}}}}

	cmd, _, _, err := runWithDeps(t, fakeEngineDeps(fe), map[string]string{"dir": dir, "dry-run": "true"})
	require.NoError(t, err)
	require.False(t, fe.executed)

	event, ok := telemetry.EventFor(cmd, nil)
	require.True(t, ok)
	assert.Equal(t, false, event.EventProperties["sync_ran"])
}

func TestTelemetry_AnnotationSet(t *testing.T) {
	cmd := Cmd()
	assert.Contains(t, cmd.Annotations, "telemetry")
}

func TestTelemetry_ExtractorPropertiesAfterFlagParse(t *testing.T) {
	cmd := Cmd()
	require.NoError(t, cmd.ParseFlags([]string{"--dry-run", "--yes", "--output-format=json"}))

	event, ok := telemetry.EventFor(cmd, nil)
	require.True(t, ok, "EventFor must return ok=true for an annotated command")

	assert.Equal(t, true, event.EventProperties["dry_run"])
	assert.Equal(t, false, event.EventProperties["diff"])
	assert.Equal(t, true, event.EventProperties["yes"])
	assert.Equal(t, "json", event.EventProperties["output_format"])
}

// The verify attribute must make --verify runs distinguishable from plain
// runs. It is emitted unconditionally (false without the flag) rather than
// omitted, so a missing key reads as a wiring bug instead of quietly
// blending verify runs into the plain-run population.
func TestTelemetry_VerifyAttribute(t *testing.T) {
	t.Run("false without the flag", func(t *testing.T) {
		cmd := Cmd()
		require.NoError(t, cmd.ParseFlags([]string{"--yes"}))

		event, ok := telemetry.EventFor(cmd, nil)
		require.True(t, ok, "EventFor must return ok=true for an annotated command")

		assert.Contains(t, event.EventProperties, "verify", "the attribute must always be emitted, never omitted")
		assert.Equal(t, false, event.EventProperties["verify"])
	})

	t.Run("true with --verify", func(t *testing.T) {
		cmd := Cmd()
		require.NoError(t, cmd.ParseFlags([]string{"--verify", "--yes"}))

		event, ok := telemetry.EventFor(cmd, nil)
		require.True(t, ok, "EventFor must return ok=true for an annotated command")

		assert.Equal(t, true, event.EventProperties["verify"])
	})
}
