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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func render(t *testing.T, s Summary, plan Plan) string {
	t.Helper()

	var b strings.Builder

	require.NoError(t, Render(&b, s, plan))

	return b.String()
}

var appSummary = Summary{Name: "my-app", WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0"}

func TestRender_Empty(t *testing.T) {
	out := render(t, appSummary, Plan{State: StateRunning, Code: builtCode(0)})

	assert.Equal(t, "my-app (68b0c1d2), running\n\n✓ Already up to date\n", out)
}

func TestRender_TheThreeClasses(t *testing.T) {
	plan := Plan{
		State:    StateRunning,
		Code:     builtCode(14),
		Artifact: []Change{{Path: "containerGroups[default].containers[primary].port", Have: 8080.0, Want: 9090.0}},
		Runtime:  []Change{{Path: "containerGroups[default].replicaCount", Have: 1.0, Want: 3.0}},
	}

	out := render(t, appSummary, plan)

	assert.Contains(t, out, "my-app (68b0c1d2), running")
	assert.Contains(t, out, "~ code       14 files changed since the last deploy")
	assert.Contains(t, out, "+ artifact   new version, 1 spec change")
	assert.Contains(t, out, "~ runtime    containerGroups[default].replicaCount: 1 -> 3")
	assert.NotContains(t, out, "Already up to date")
}

// TestRender_SingleRuntimeChangeSitsOnItsOwnLine: the common case is one
// number moving, and pushing it onto a sub-line for consistency's sake would
// cost a line to say nothing.
func TestRender_SingleRuntimeChangeSitsOnItsOwnLine(t *testing.T) {
	out := render(t, appSummary, Plan{
		State:   StateRunning,
		Runtime: []Change{{Path: "containerGroups[default].replicaCount", Have: 1.0, Want: 3.0}},
	})

	assert.Contains(t, out, "~ runtime    containerGroups[default].replicaCount: 1 -> 3")
	assert.NotContains(t, out, "2 changes")
}

func TestRender_SeveralRuntimeChangesAreListed(t *testing.T) {
	out := render(t, appSummary, Plan{
		State: StateRunning,
		Runtime: []Change{
			{Path: "containerGroups[default].replicaCount", Have: 1.0, Want: 3.0},
			{Path: "containerGroups[default].containers[primary].resourceAllocation.cpu", Have: 0.5, Want: 2.0},
		},
	})

	assert.Contains(t, out, "~ runtime    2 changes")
	assert.Contains(t, out, "      containerGroups[default].replicaCount: 1 -> 3")
	assert.Contains(t, out, "      containerGroups[default].containers[primary].resourceAllocation.cpu: 0.5 -> 2")
}

func TestRender_FirstDeploy(t *testing.T) {
	out := render(t, Summary{Name: "my-app"}, Plan{
		State:   StateUnbound,
		Creates: true,
		Code:    CodeChange{Applies: true, FirstDeploy: true},
	})

	assert.Contains(t, out, "my-app, no workload bound yet")
	assert.NotContains(t, out, "(", "there is no id to show yet")
	assert.Contains(t, out, "+ workload   new, with its first artifact")
	assert.Contains(t, out, "~ code       the whole project, uploaded for the first time")
}

// TestRender_PublishedImageNeverMentionsCode: a manifest naming an image has
// no code to sync, and a "0 files changed" line would invite the reader to
// expect a rebuild that is never going to happen.
func TestRender_PublishedImageNeverMentionsCode(t *testing.T) {
	out := render(t, appSummary, Plan{
		State:    StateRunning,
		Code:     CodeChange{Applies: false, Files: 12},
		Artifact: []Change{{Path: "containerGroups[default].containers[primary].imageUri", Have: "a:1", Want: "a:2"}},
	})

	assert.NotContains(t, out, "code")
	assert.Contains(t, out, "+ artifact")
}

// TestRender_ArtifactFromCodeAloneSaysWhy covers a rebuild with no spec
// change: the version is new because the code is, and the plan has to say so
// rather than print an artifact entry with nothing under it.
func TestRender_ArtifactFromCodeAloneSaysWhy(t *testing.T) {
	out := render(t, appSummary, Plan{State: StateRunning, Code: builtCode(3)})

	assert.Contains(t, out, "+ artifact   rebuilt from the synced code")
}

// TestRender_NeverPrintsEnvironmentVariableValues is the one hard rule in
// here. A literal can be a secret someone pasted in plaintext, and a plan
// that echoed it would put it in scrollback and CI logs.
func TestRender_NeverPrintsEnvironmentVariableValues(t *testing.T) {
	const secret = "sk-abcdefghijklmnopqrstuvwxyz0123"

	plan := Plan{
		State: StateRunning,
		Artifact: []Change{
			{
				Path: "containerGroups[default].containers[primary].environmentVars[OPENAI_API_KEY].value",
				Have: "sk-oldvalue", Want: secret,
			},
			{
				Path:   "containerGroups[default].containers[primary].environmentVars[NEW_TOKEN]",
				Absent: true, Want: map[string]any{"name": "NEW_TOKEN", "value": secret},
			},
		},
	}

	out := render(t, appSummary, plan)

	assert.NotContains(t, out, secret)
	assert.NotContains(t, out, "sk-oldvalue")
	assert.Contains(t, out, "environmentVars[OPENAI_API_KEY].value: changed")
	assert.Contains(t, out, "environmentVars[NEW_TOKEN]: set")
	assert.Contains(t, out, "OPENAI_API_KEY", "the name is the whole point of showing the line")
}

// TestRender_CapsTheDetailListOutLoud: a truncated plan that did not say it
// was truncated would read as a complete account of what is about to happen.
func TestRender_CapsTheDetailListOutLoud(t *testing.T) {
	changes := make([]Change, 0, 10)
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		changes = append(changes, Change{Path: "spec." + name, Have: 1.0, Want: 2.0})
	}

	out := render(t, appSummary, Plan{State: StateRunning, Artifact: changes})

	assert.Contains(t, out, "+ artifact   new version, 10 spec changes")
	assert.Contains(t, out, "      spec.a: 1 -> 2")
	assert.Contains(t, out, "      spec.f: 1 -> 2")
	assert.NotContains(t, out, "spec.g")
	assert.Contains(t, out, "and 4 more")
}

// A stopped workload with nothing else to change is the one plan whose reason
// to act is the state rather than a difference, so the body has to say so:
// a header and nothing under it reads as a plan to do nothing, which would
// disagree with the "started" the JSON envelope reports.
func TestRender_StoppedWorkloadWithNothingToChange(t *testing.T) {
	plan := Plan{State: StateStopped, Code: builtCode(0)}
	out := render(t, appSummary, plan)

	assert.Contains(t, out, "my-app (68b0c1d2), stopped")
	assert.Contains(t, out, "~ workload")
	assert.Contains(t, out, "started")
	assert.NotContains(t, out, "Already up to date", "the user asked for it to be running")
	assert.Equal(t, ActionStarted, plan.Action(), "the text and the envelope have to agree")
}

// The start line is about the workload's state, so it survives alongside the
// differences rather than only appearing when there are none.
func TestRender_StoppedWorkloadWithDriftSaysBoth(t *testing.T) {
	out := render(t, appSummary, Plan{
		State:   StateStopped,
		Runtime: []Change{{Path: "containerGroups[default].replicaCount", Have: 1.0, Want: 3.0}},
	})

	assert.Contains(t, out, "started")
	assert.Contains(t, out, "containerGroups[default].replicaCount: 1 -> 3")
}

func TestRender_ShortIDLeavesShortIDsAlone(t *testing.T) {
	out := render(t, Summary{Name: "my-app", WorkloadID: "abc"}, Plan{State: StateRunning, Code: builtCode(1)})

	assert.Contains(t, out, "my-app (abc), running")
}

func TestRender_UnnamedWorkloadStillRenders(t *testing.T) {
	out := render(t, Summary{}, Plan{State: StateUnbound, Creates: true})

	assert.Contains(t, out, "workload, no workload bound yet")
}

func TestPlanJSON_Shape(t *testing.T) {
	plan := Plan{
		State:    StateRunning,
		Code:     builtCode(14),
		Artifact: []Change{{Path: "containerGroups[default].containers[primary].port", Have: 8080.0, Want: 9090.0}},
		Runtime:  []Change{{Path: "containerGroups[default].replicaCount", Have: 1.0, Want: 3.0}},
	}

	encoded, err := json.Marshal(plan.JSON())
	require.NoError(t, err)

	var decoded map[string]any

	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.Equal(t, "rolled", decoded["action"])
	assert.Equal(t, "running", decoded["state"])
	assert.Equal(t, false, decoded["creates"])

	code, _ := decoded["code"].(map[string]any)
	assert.Equal(t, true, code["changed"])
	assert.InDelta(t, 14.0, code["files"], 0)

	artifact, _ := decoded["artifact"].([]any)
	assert.Len(t, artifact, 1)
}

// TestPlanJSON_EmptyListsAreNotNull keeps a consumer from having to special
// case null: an empty plan has empty arrays, not missing ones.
func TestPlanJSON_EmptyListsAreNotNull(t *testing.T) {
	encoded, err := json.Marshal(Plan{State: StateRunning}.JSON())
	require.NoError(t, err)

	assert.Contains(t, string(encoded), `"artifact":[]`)
	assert.Contains(t, string(encoded), `"runtime":[]`)
	assert.Contains(t, string(encoded), `"action":"unchanged"`)
}

// TestPlanJSON_RedactsToo: the machine-readable plan is the one that ends up
// in CI artifacts, so it cannot be the leaky one.
func TestPlanJSON_RedactsToo(t *testing.T) {
	const secret = "sk-abcdefghijklmnopqrstuvwxyz0123"

	plan := Plan{
		State: StateRunning,
		Artifact: []Change{{
			Path: "containerGroups[default].containers[primary].environmentVars[OPENAI_API_KEY].value",
			Have: "old", Want: secret,
		}},
	}

	encoded, err := json.Marshal(plan.JSON())
	require.NoError(t, err)

	assert.NotContains(t, string(encoded), secret)
	assert.Contains(t, string(encoded), "OPENAI_API_KEY")
}
