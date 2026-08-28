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
	"fmt"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renderDiff is the diff-mode twin of render: it runs one plan through
// RenderDiff and hands back the bytes, so the tests below can assert on
// output the way render_test.go does.
func renderDiff(t *testing.T, s Summary, plan Plan) string {
	t.Helper()

	var b strings.Builder

	require.NoError(t, RenderDiff(&b, s, plan))

	return b.String()
}

// TestRenderDiff_SizingChangeIsAUnifiedDiff is the shape the flag exists for:
// a moved value states both sides of itself, with the file's value leaving
// and the manifest's value arriving, where the default plan would print one
// "have -> want" line.
func TestRenderDiff_SizingChangeIsAUnifiedDiff(t *testing.T) {
	plan := Plan{
		State: StateRunning,
		Runtime: []Change{
			{Path: "containerGroups[default].replicaCount", Have: 1.0, Want: 3.0},
		},
		DiffRuntime: []DiffRow{
			{Path: "containerGroups[default].replicaCount", Want: 3.0, Have: 1.0, Changed: true},
		},
	}

	out := renderDiff(t, appSummary, plan)

	assert.Contains(t, out, "- containerGroups[default].replicaCount: 1")
	assert.Contains(t, out, "+ containerGroups[default].replicaCount: 3")
}

// TestRenderDiff_CollapsesUnchangedRunsBeyondTheWindow pins the git
// convention the renderer inherits from uidiff: agreeing leaves stay visible
// within three lines of a change and collapse into a counted summary beyond
// it, so a big spec still renders shorter than itself without ever hiding a
// change.
func TestRenderDiff_CollapsesUnchangedRunsBeyondTheWindow(t *testing.T) {
	rows := make([]DiffRow, 0, 13)

	for i := 1; i <= 6; i++ {
		rows = append(rows, DiffRow{Path: fmt.Sprintf("spec.leaf%02d", i), Want: float64(i), Have: float64(i)})
	}

	rows = append(rows, DiffRow{Path: "spec.port", Want: 9090.0, Have: 8080.0, Changed: true})

	for i := 7; i <= 12; i++ {
		rows = append(rows, DiffRow{Path: fmt.Sprintf("spec.leaf%02d", i), Want: float64(i), Have: float64(i)})
	}

	// The Change list rides along because Build always fills it beside the
	// rows, and plan.Empty() reads it rather than the rows: the diff's
	// verdict has to mean what the run's exit code means.
	out := renderDiff(t, appSummary, Plan{
		State:        StateRunning,
		Artifact:     []Change{{Path: "spec.port", Have: 8080.0, Want: 9090.0}},
		DiffArtifact: rows,
	})

	// Three unchanged leaves on the far side of each window edge hide; the
	// summary counts them out loud rather than leaving the reader to guess.
	assert.Equal(t, 2, strings.Count(out, "... 3 identical lines"))
	assert.NotContains(t, out, "spec.leaf01", "beyond the window, context collapses")
	assert.NotContains(t, out, "spec.leaf03")
	assert.Contains(t, out, "spec.leaf04", "the row at the window edge stays")
	assert.Contains(t, out, "spec.leaf09", "the last row inside the far window stays")
	assert.NotContains(t, out, "spec.leaf10")
}

// TestRenderDiff_NeverTruncates is the other half of what --diff is bought
// for: the default plan caps its detail list at detailLimit and counts the
// rest, because a plan is a summary. A diff is the detail, so every changed
// leaf appears no matter how many there are.
func TestRenderDiff_NeverTruncates(t *testing.T) {
	rows := make([]DiffRow, 0, 10)

	changes := make([]Change, 0, 10)

	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		rows = append(rows, DiffRow{Path: "spec." + name, Want: 2.0, Have: 1.0, Changed: true})

		changes = append(changes, Change{Path: "spec." + name, Have: 1.0, Want: 2.0})
	}

	out := renderDiff(t, appSummary, Plan{State: StateRunning, Artifact: changes, DiffArtifact: rows})

	assert.Contains(t, out, "- spec.j: 1")
	assert.Contains(t, out, "+ spec.j: 2")
	assert.NotContains(t, out, "more", "nothing is dropped, so nothing is counted out loud")
}

// TestRenderDiff_NeverPrintsEnvironmentVariableValues is the hard rule, run
// across all three row states at once: a literal that moved, a literal that
// agrees, and a credential reference that rotated. The values behind any
// `.environmentVars[` path stay out of the output entirely -- additions,
// removals and context alike -- while the variable names stay visible,
// because the names are what tell the reader what changed.
func TestRenderDiff_NeverPrintsEnvironmentVariableValues(t *testing.T) {
	const (
		literalOld  = "sk-live-abc123"
		literalNew  = "sk-live-xyz789"
		literalSame = "hunter2"
		credOld     = "66f1a2b3c4d5e6f7a8b9c0d1"
		credNew     = "aaaaaaaaaaaaaaaaaaaaaaaa"
	)

	plan := Plan{
		State: StateRunning,
		Artifact: []Change{
			{
				Path: "containerGroups[default].containers[primary].environmentVars[OPENAI_API_KEY].value",
				Have: literalOld,
				Want: literalNew,
			},
			{
				Path: "containerGroups[default].containers[primary].environmentVars[HUGGING_FACE_HUB_TOKEN].drCredentialId",
				Have: credOld,
				Want: credNew,
			},
		},
		DiffArtifact: []DiffRow{
			{
				Path:    "containerGroups[default].containers[primary].environmentVars[OPENAI_API_KEY].value",
				Have:    literalOld,
				Want:    literalNew,
				Changed: true,
			},
			{
				Path: "containerGroups[default].containers[primary].environmentVars[PLAINTEXT].value",
				Have: literalSame,
				Want: literalSame,
			},
			{
				Path:    "containerGroups[default].containers[primary].environmentVars[HUGGING_FACE_HUB_TOKEN].drCredentialId",
				Have:    credOld,
				Want:    credNew,
				Changed: true,
			},
			{
				Path: "containerGroups[default].containers[primary].environmentVars[FOO].name",
				Have: "FOO",
				Want: "FOO",
			},
		},
	}

	out := renderDiff(t, appSummary, plan)

	for _, secret := range []string{literalOld, literalNew, literalSame, credOld, credNew} {
		assert.NotContains(t, out, secret)
	}

	assert.Contains(t, out, "environmentVars[OPENAI_API_KEY].value: changed")
	assert.Contains(t, out, "environmentVars[OPENAI_API_KEY].value: set")
	assert.Contains(t, out, "environmentVars[PLAINTEXT].value: (redacted)")
	assert.Contains(t, out, "environmentVars[HUGGING_FACE_HUB_TOKEN].drCredentialId: changed")
	assert.Contains(t, out, "environmentVars[FOO].name: (redacted)",
		"a name leaf inside the block sits on a redacted path, so its value is withheld with the rest")
}

// TestRenderDiff_NameLeavesRenderAsContext records the render-time choice
// about the name leaves DiffRows emits mechanically. They stay: inside a
// container's run of rows, the name leaf is the only line that says which
// element the surrounding lines belong to, and dropping it would leave a
// two-container diff ambiguous. They are context, never changes.
func TestRenderDiff_NameLeavesRenderAsContext(t *testing.T) {
	plan := Plan{
		State: StateRunning,
		Artifact: []Change{
			{Path: "containerGroups[default].containers[primary].port", Have: 8080.0, Want: 9090.0},
		},
		DiffArtifact: []DiffRow{
			{Path: "containerGroups[default].containers[primary].name", Have: "primary", Want: "primary"},
			{Path: "containerGroups[default].containers[primary].port", Have: 8080.0, Want: 9090.0, Changed: true},
		},
	}

	out := renderDiff(t, appSummary, plan)

	assert.Contains(t, out, "containerGroups[default].containers[primary].name: primary")
	assert.NotContains(t, out, "- containerGroups[default].containers[primary].name")
}

// TestRenderDiff_MultiBlockChangesRenderInOneDiff: a plan that moves the
// artifact spec and the runtime sizing renders both halves in one diff,
// because they are one deploy and the reader reviews them together.
func TestRenderDiff_MultiBlockChangesRenderInOneDiff(t *testing.T) {
	drifted := `{
	  "name": "my-app",
	  "artifact": {"name": "my-app-artifact", "spec": {
	    "type": "service",
	    "containerGroups": [{"name": "default", "containers": [
	      {"name": "primary", "primary": true, "port": 9090,
	       "readinessProbe": {"path": "/health", "port": 8080}}
	    ]}]
	  }},
	  "runtime": {"containerGroups": [
	    {"name": "default", "replicaCount": 3,
	     "containers": [{"name": "primary", "resourceAllocation": {"cpu": 0.5, "memory": "512MB"}}]}
	  ]}
	}`

	plan, err := Build(
		loadedFrom(drifted),
		liveFrom(t, StateRunning, planLiveSpec, planLiveRuntime),
		builtCode(0),
		Options{},
	)
	require.NoError(t, err)

	out := renderDiff(t, appSummary, plan)

	assert.Contains(t, out, "- containerGroups[default].containers[primary].port: 8080")
	assert.Contains(t, out, "+ containerGroups[default].containers[primary].port: 9090")
	assert.Contains(t, out, "- containerGroups[default].replicaCount: 1")
	assert.Contains(t, out, "+ containerGroups[default].replicaCount: 3")
}

// TestRenderDiff_AlreadyUpToDateIsTheDefaultVerdict: an empty plan with
// nothing unmanaged has nothing for a diff to draw, so --diff prints exactly
// what the default mode prints. The run's outcome, and its exit code, are
// the same run's.
func TestRenderDiff_AlreadyUpToDateIsTheDefaultVerdict(t *testing.T) {
	plan := Plan{State: StateRunning, Code: builtCode(0)}

	out := renderDiff(t, appSummary, plan)

	assert.Equal(t, render(t, appSummary, plan), out)
	assert.Contains(t, out, "✓ Already up to date")
}

// TestRenderDiff_StateLinesRenderAsActionLines: stopped, errored and
// terminated are reasons the run will act, not field changes, so they keep
// the entry form and position the default plan gives them. Inside a diff
// they must not read as edits to fields nobody wrote.
func TestRenderDiff_StateLinesRenderAsActionLines(t *testing.T) {
	tests := []struct {
		name string
		plan Plan
		want string
	}{
		{"stopped", Plan{State: StateStopped, Code: builtCode(0)}, "~ workload"},
		{"errored", Plan{State: StateErrored}, "! workload"},
		{"terminated", Plan{State: StateTerminated}, "! workload"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := renderDiff(t, appSummary, tc.plan)

			assert.Contains(t, out, "  "+tc.want, "an entry line, indented like the default plan's")
			assert.NotContains(t, out, "\n- "+tc.want)
			assert.NotContains(t, out, "\n+ "+tc.want)
		})
	}
}

// The state line survives alongside the field rows, and the rows still
// render as rows: the start and the sizing move in the same diff because
// they happen in the same deploy.
func TestRenderDiff_StoppedWorkloadRendersStartAndRowsTogether(t *testing.T) {
	plan := Plan{
		State: StateStopped,
		DiffRuntime: []DiffRow{
			{Path: "containerGroups[default].replicaCount", Want: 3.0, Have: 1.0, Changed: true},
		},
	}

	out := renderDiff(t, appSummary, plan)

	assert.Contains(t, out, "started, having been stopped")
	assert.Contains(t, out, "- containerGroups[default].replicaCount: 1")
	assert.Contains(t, out, "+ containerGroups[default].replicaCount: 3")
}

// TestRenderDiff_LockLineRendersAsAnActionLine: locking is a consequence of
// the deploy, not an edit to a field, so the ~ entry keeps the form and
// position the default plan gives it and never becomes a hunk.
func TestRenderDiff_LockLineRendersAsAnActionLine(t *testing.T) {
	plan := Plan{
		State:  StateRunning,
		Locked: true,
		Code:   builtCode(1),
		DiffArtifact: []DiffRow{
			{Path: "port", Want: 9090.0, Have: 8080.0, Changed: true},
		},
	}

	out := renderDiff(t, appSummary, plan)

	assert.Contains(t, out, "~ lock")
	assert.Contains(t, out, "locked to match")
	assert.NotContains(t, out, "- lock")
	assert.NotContains(t, out, "+ lock")
}

// TestRenderDiff_UnmanagedNeverBecomesARemoval is the three-state rule's
// sharp edge. What the live object carries that the file never names renders
// once, as a counted summary; it must never appear as a `-` line, because
// the file leaving a field out is the file declining to manage it, not
// asking for it to be deleted.
func TestRenderDiff_UnmanagedNeverBecomesARemoval(t *testing.T) {
	plan := Plan{
		State: StateRunning,
		Runtime: []Change{
			{Path: "containerGroups[default].replicaCount", Have: 1.0, Want: 3.0},
		},
		DiffRuntime: []DiffRow{
			{Path: "containerGroups[default].replicaCount", Want: 3.0, Have: 1.0, Changed: true},
		},
		Unmanaged: []string{
			"containerGroups[default].containers[metrics]",
			"containerGroups[default].containers[primary].environmentVars[FOO]",
		},
	}

	out := renderDiff(t, appSummary, plan)

	assert.Contains(t, out, "2 fields not managed by this file")
	assert.NotContains(t, out, "- containerGroups[default].containers[metrics]")
	assert.NotContains(t, out, "- containerGroups[default].containers[primary].environmentVars[FOO]")
	assert.Equal(t, 1, strings.Count(out, "not managed by this file"),
		"the summary is stated once, never once per field")
}

// TestRenderDiff_UnmanagedSummarySaysFieldOnce: the count goes through the
// same plural helper as the rest of the plan, so a lone extra does not read
// like a census.
func TestRenderDiff_UnmanagedSummarySaysFieldOnce(t *testing.T) {
	plan := Plan{
		State:     StateRunning,
		Code:      builtCode(0),
		Unmanaged: []string{"containerGroups[default].containers[metrics]"},
	}

	out := renderDiff(t, appSummary, plan)

	assert.Contains(t, out, "1 field not managed by this file")
}

// TestRenderDiff_EmptyPlanWithUnmanagedSurfacesThem is where --diff
// deliberately diverges from the default mode: Render short-circuits an
// empty plan and never mentions unmanaged fields, while the diff still says
// what the live object carries that the file never names. The verdict, and
// the exit code behind it, are unchanged.
func TestRenderDiff_EmptyPlanWithUnmanagedSurfacesThem(t *testing.T) {
	plan := Plan{
		State: StateRunning,
		Code:  builtCode(0),
		Unmanaged: []string{
			"containerGroups[default].containers[metrics]",
			"containerGroups[default].containers[primary].environmentVars[FOO]",
		},
	}

	out := renderDiff(t, appSummary, plan)

	assert.Contains(t, out, "✓ Already up to date")
	assert.Contains(t, out, "2 fields not managed by this file")
	assert.NotContains(t, out, "\n- ")
	assert.NotContains(t, out, "\n+ ", "no managed field changed, so no change lines exist")
}

// TestRenderDiff_FirstDeployIsAllAdditions: with no live side to compare
// against, the diff's subject is the compiled manifest itself. A header says
// so, the create summary line the default plan prints still leads, and every
// leaf of the file renders as an addition -- the diff must not be less
// informative than the plan it replaces.
func TestRenderDiff_FirstDeployIsAllAdditions(t *testing.T) {
	plan, err := Build(
		loadedFrom(planPayload),
		Live{State: StateUnbound},
		CodeChange{Applies: true, FirstDeploy: true},
		Options{},
	)
	require.NoError(t, err)

	out := renderDiff(t, Summary{Name: "my-app"}, plan)

	assert.Contains(t, out, "my-app, first deploy")
	assert.Contains(t, out, "+ workload   my-app will be created, with its first artifact")
	assert.Contains(t, out, "~ code       all project files, uploaded for the first time")
	assert.Contains(t, out, "+ containerGroups[default].containers[primary].port: 8080")
	assert.Contains(t, out, "+ containerGroups[default].replicaCount: 1")
	assert.NotContains(t, out, "\n- ", "there is nothing live to remove anything from")
}

// A create that replaces a dead binding is not a first deploy, and the
// header must not claim it is: the create line's dead-binding warning is the
// whole point of that plan, exactly as in the default mode.
func TestRenderDiff_RecreateDoesNotClaimFirstDeploy(t *testing.T) {
	plan, err := Build(
		loadedFrom(planPayload),
		Live{Live: manifest.Live{WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0"}, State: StateMissing},
		builtCode(0),
		Options{},
	)
	require.NoError(t, err)

	out := renderDiff(t, Summary{Name: "my-app"}, plan)

	assert.Contains(t, out, "+ workload   my-app will be created: .datarobot.yaml is bound to 68b0c1d2, "+
		"which no longer exists")
	assert.NotContains(t, out, "first deploy")
}

// TestRenderDiff_ArtifactEntryAnnouncesTheRoll: the one fact no field value
// carries is that a new version will be minted and rolled. The default
// plan's artifact entry keeps its place above the diff; its capped detail
// list is what the rows below it replace.
func TestRenderDiff_ArtifactEntryAnnouncesTheRoll(t *testing.T) {
	plan := Plan{
		State: StateRunning,
		Code:  builtCode(0),
		DiffArtifact: []DiffRow{
			{Path: "containerGroups[default].containers[primary].port", Want: 9090.0, Have: 8080.0, Changed: true},
		},
	}

	plan.Artifact = []Change{{Path: "containerGroups[default].containers[primary].port", Have: 8080.0, Want: 9090.0}}

	out := renderDiff(t, appSummary, plan)

	assert.Contains(t, out, "+ artifact   new version, 1 spec change")
	assert.NotContains(t, out, "      containerGroups[default].containers[primary].port",
		"the capped detail list is replaced by the diff, not repeated under it")
}

// TestRenderDiff_CodeLineKeepsItsPositionUntilTheFileListLands: code drift
// is measured upstream of the renderer, so until the sync file list replaces
// it the bare count renders as the action line it already is.
func TestRenderDiff_CodeLineKeepsItsPositionUntilTheFileListLands(t *testing.T) {
	plan := Plan{
		State: StateRunning,
		Code:  builtCode(14),
		DiffArtifact: []DiffRow{
			{Path: "port", Want: 9090.0, Have: 8080.0, Changed: true},
		},
	}

	out := renderDiff(t, appSummary, plan)

	assert.Contains(t, out, "~ code       14 files changed since the last deploy")
}

// TestRender_DefaultModeBaselineIsUnchanged pins the default plan's exact
// bytes against a baseline captured before --diff existed. The renderer grew
// fields that carry the diff rows, and this is the proof that Render neither
// reads them nor moved: same header, same capped detail list with its
// "and 1 more", same line order, byte for byte.
func TestRender_DefaultModeBaselineIsUnchanged(t *testing.T) {
	changes := make([]Change, 0, 7)

	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		changes = append(changes, Change{Path: "spec." + name, Have: 1.0, Want: 2.0})
	}

	plan := Plan{
		State:    StateRunning,
		Code:     builtCode(14),
		Locked:   true,
		Artifact: changes,
		Runtime:  []Change{{Path: "containerGroups[default].replicaCount", Have: 1.0, Want: 3.0}},
		DiffArtifact: []DiffRow{
			{Path: "spec.a", Want: 2.0, Have: 1.0, Changed: true},
		},
		DiffRuntime: []DiffRow{
			{Path: "containerGroups[default].replicaCount", Want: 3.0, Have: 1.0, Changed: true},
		},
		Unmanaged: []string{"containerGroups[default].containers[metrics]"},
	}

	assert.Equal(t,
		"my-app (68b0c1d2), running\n"+
			"\n"+
			"  ~ code       14 files changed since the last deploy\n"+
			"  ~ lock       the running version is locked, so a new one is created and locked to match. "+
			"Locking is permanent\n"+
			"  + artifact   new version, 7 spec changes\n"+
			"      spec.a: 1 -> 2\n"+
			"      spec.b: 1 -> 2\n"+
			"      spec.c: 1 -> 2\n"+
			"      spec.d: 1 -> 2\n"+
			"      spec.e: 1 -> 2\n"+
			"      spec.f: 1 -> 2\n"+
			"      and 1 more\n"+
			"  ~ runtime    containerGroups[default].replicaCount: 1 -> 3\n",
		render(t, appSummary, plan))
}
