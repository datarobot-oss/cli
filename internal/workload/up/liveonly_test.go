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
	"encoding/json"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// liveWithEnvVar is planLiveSpec with a variable on the primary container
// that the file never names, beside the sidecar it never names either.
const liveWithEnvVar = `{
  "type": "service",
  "containerGroups": [
    {"name": "default", "containers": [
      {"name": "primary", "primary": true, "port": 8080,
       "readinessProbe": {"path": "/health", "port": 8080},
       "environmentVars": [{"name": "LEGACY_FLAG", "value": "s3cr3t-value"}]},
      {"name": "metrics", "imageUri": "registry/metrics:v1"}
    ]}
  ]
}`

// resizedPayload moves only the replica count, so the plan resizes without
// rolling a version.
const resizedPayload = `{
  "name": "my-app",
  "artifact": {
    "name": "my-app-artifact",
    "spec": {
      "type": "service",
      "containerGroups": [
        {"name": "default", "containers": [
          {"name": "primary", "primary": true, "port": 8080,
           "readinessProbe": {"path": "/health", "port": 8080}}
        ]}
      ]
    }
  },
  "runtime": {
    "containerGroups": [
      {"name": "default", "replicaCount": 3,
       "containers": [{"name": "primary", "resourceAllocation": {"cpu": 0.5, "memory": "512MB"}}]}
    ]
  }
}`

// rolledPayload moves the port, so the plan rolls a version and writes the
// file's spec whole.
const rolledPayload = `{
  "name": "my-app",
  "artifact": {
    "name": "my-app-artifact",
    "spec": {
      "type": "service",
      "containerGroups": [
        {"name": "default", "containers": [
          {"name": "primary", "primary": true, "port": 9090,
           "readinessProbe": {"path": "/health", "port": 8080}}
        ]}
      ]
    }
  },
  "runtime": {
    "containerGroups": [
      {"name": "default", "replicaCount": 1,
       "containers": [{"name": "primary", "resourceAllocation": {"cpu": 0.5, "memory": "512MB"}}]}
    ]
  }
}`

func removedRows(rows []DiffRow) []DiffRow {
	var out []DiffRow

	for _, r := range rows {
		if r.Removed {
			out = append(out, r)
		}
	}

	return out
}

// A plan that finds nothing to do leaves every live-only element alone, so
// they are unmanaged and none of them is a removal.
func TestBuild_EmptyPlanLeavesLiveOnlyElementsAlone(t *testing.T) {
	plan, err := Build(
		loadedFrom(planPayload),
		liveFrom(t, StateRunning, liveWithEnvVar, planLiveRuntime),
		builtCode(0),
		Options{},
	)
	require.NoError(t, err)
	require.True(t, plan.Empty())

	assert.Equal(t, []string{
		"containerGroups[default].containers[primary].environmentVars[LEGACY_FLAG]",
		"containerGroups[default].containers[metrics]",
	}, plan.Unmanaged)
	assert.Empty(t, removedRows(plan.DiffArtifact))
	assert.Empty(t, removedRows(plan.DiffRuntime))
}

// A resize sends the file's runtime block whole, so the sidecar's sizing is
// dropped and shows as a removal; the spec is not written, so the sidecar's
// spec and the variable survive as unmanaged.
func TestBuild_ResizeDropsOnlyTheRuntimeSide(t *testing.T) {
	plan, err := Build(
		loadedFrom(resizedPayload),
		liveFrom(t, StateRunning, liveWithEnvVar, planLiveRuntime),
		builtCode(0),
		Options{},
	)
	require.NoError(t, err)
	require.True(t, plan.Retunes())

	assert.Equal(t, []string{
		"containerGroups[default].containers[primary].environmentVars[LEGACY_FLAG]",
		"containerGroups[default].containers[metrics]",
	}, plan.Unmanaged)
	assert.Empty(t, removedRows(plan.DiffArtifact))

	removed := removedRows(plan.DiffRuntime)
	require.Len(t, removed, 1)
	assert.Equal(t, "containerGroups[default].containers[metrics]", removed[0].Path)
	assert.Nil(t, removed[0].Want)
	assert.NotNil(t, removed[0].Have)
}

// A roll writes the file's spec whole, so both the sidecar and the variable
// are dropped from the artifact. The runtime is not sent on a roll that
// moves no sizing, so the sidecar's sizing survives.
func TestBuild_RollDropsTheSpecSide(t *testing.T) {
	plan, err := Build(
		loadedFrom(rolledPayload),
		liveFrom(t, StateRunning, liveWithEnvVar, planLiveRuntime),
		builtCode(0),
		Options{},
	)
	require.NoError(t, err)
	require.True(t, plan.RollsArtifact())

	assert.Equal(t, []string{"containerGroups[default].containers[metrics]"}, plan.Unmanaged,
		"the runtime side is left alone")
	assert.Empty(t, removedRows(plan.DiffRuntime))

	removed := removedRows(plan.DiffArtifact)
	require.Len(t, removed, 2)
	assert.Equal(t, "containerGroups[default].containers[primary].environmentVars[LEGACY_FLAG]", removed[0].Path)
	assert.Equal(t, "containerGroups[default].containers[metrics]", removed[1].Path)
}

// A file bound by artifact id names no spec, so a roll onto another version
// writes none and drops nothing.
func TestBuild_IDBoundRollDropsNothing(t *testing.T) {
	loaded := Loaded{Compiled: &manifest.Compiled{
		Payload:    json.RawMessage(`{"name": "my-app", "artifactId": "68b0bbbb0000000000000002"}`),
		ArtifactID: "68b0bbbb0000000000000002",
	}}

	live := liveFrom(t, StateRunning, planLiveSpec, planLiveRuntime)
	live.ArtifactID = "68a0000000000000000000a1"

	plan, err := Build(loaded, live, builtCode(0), Options{})
	require.NoError(t, err)
	require.True(t, plan.RollsArtifact())

	assert.Empty(t, removedRows(plan.DiffArtifact))
	assert.Equal(t, []string{"containerGroups[default]"}, plan.Unmanaged,
		"the file manages nothing of either block, so the whole group is left alone")
}

// The diff says what a roll drops as `-` lines, and the variable's value
// never prints.
func TestRenderDiff_RollShowsDroppedElementsAsRemovals(t *testing.T) {
	plan, err := Build(
		loadedFrom(rolledPayload),
		liveFrom(t, StateRunning, liveWithEnvVar, planLiveRuntime),
		builtCode(0),
		Options{},
	)
	require.NoError(t, err)

	out := renderDiff(t, appSummary, plan)

	assert.Contains(t, out, "- containerGroups[default].containers[metrics]: {imageUri, name}")
	assert.Contains(t, out, "- containerGroups[default].containers[primary].environmentVars[LEGACY_FLAG]: removed",
		"a dropped variable reads as removed, not changed")
	assert.NotContains(t, out, "s3cr3t-value", "the variable's value is redacted")
	assert.Contains(t, out, "1 field not managed by this file", "the sidecar's sizing is left alone")
}

// The JSON diff carries a dropped element as a change marked removed, with
// the live element as have and no want, and lists only what survives as
// unmanaged.
func TestPlanJSON_DroppedElementsAreRemovedChanges(t *testing.T) {
	plan, err := Build(
		loadedFrom(resizedPayload),
		liveFrom(t, StateRunning, planLiveSpec, planLiveRuntime),
		builtCode(0),
		Options{},
	)
	require.NoError(t, err)

	diff := plan.JSONWithDiff().Diff
	require.NotNil(t, diff)

	var removed []ChangeJSON

	for _, c := range diff.Changes {
		if c.Removed {
			removed = append(removed, c)
		}
	}

	require.Len(t, removed, 1)
	assert.Equal(t, "containerGroups[default].containers[metrics]", removed[0].Path)
	assert.NotNil(t, removed[0].Have)
	assert.Nil(t, removed[0].Want)
	assert.False(t, removed[0].Absent)
	assert.Equal(t, []string{"containerGroups[default].containers[metrics]"}, diff.Unmanaged)
}

// A refused plan is described, not announced, in the diff as in the default
// mode: the disclaimer comes before the rows.
func TestRenderDiff_RefusedPlanCarriesTheDisclaimer(t *testing.T) {
	plan := Plan{
		State: StateTerminated,
		Runtime: []Change{
			{Path: "containerGroups[default].replicaCount", Have: 1.0, Want: 3.0},
		},
		DiffRuntime: []DiffRow{
			{Path: "containerGroups[default].replicaCount", Want: 3.0, Have: 1.0, Changed: true},
		},
	}

	out := renderDiff(t, Summary{Name: "my-app", WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0", Refused: true}, plan)

	assert.Contains(t, out, "Nothing below will be applied")
	assert.Less(t, strings.Index(out, "Nothing below will be applied"), strings.Index(out, "- containerGroups"))
}

// The code section lists what the deploy pushes and nothing else: the
// downloads and conflicts the dry run measured are not its work.
func TestRenderDiff_CodeBlockListsOnlyWhatTheDeployPushes(t *testing.T) {
	plan := Plan{
		State: StateRunning,
		Code: CodeChange{
			Applies: true,
			Files:   1,
			SyncPlan: &sync.SyncPlan{
				Uploads:   []sync.FileAction{{Path: "app.py", Classification: sync.ClsLocalModified, Action: sync.ActUploadModify, LocalSize: 10}},
				Downloads: []sync.FileAction{{Path: "remote-only.txt", Classification: sync.ClsRemoteModified, Action: sync.ActDownloadModify, RemoteSize: 99}},
				Conflicts: []sync.FileAction{{Path: "both.py", Classification: sync.ClsConflict, Action: sync.ActDownloadModify, RemoteSize: 5}},
			},
		},
	}

	out := renderDiff(t, appSummary, plan)

	assert.Contains(t, out, "app.py")
	assert.NotContains(t, out, "remote-only.txt")
	assert.NotContains(t, out, "both.py")
	assert.NotContains(t, out, "DOWNLOAD")
	assert.NotContains(t, out, "CONFLICT")
}
