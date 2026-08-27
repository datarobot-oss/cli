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

// A suspended workload must not be previewed as about to start. The platform
// ignores a start while it is in that state, so the apply refuses it, and a dry
// run never gets as far as the refusal: the plan is the only thing the reader
// sees. Stopped, suspended and interrupted reduce to one state, which is why
// the line needs the status as well.
func TestRender_SuspendedIsNotPreviewedAsAStart(t *testing.T) {
	out := render(t,
		Summary{Name: "my-app", WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0", Status: "suspended"},
		Plan{State: StateStopped})

	assert.Contains(t, out, "suspended, which a deploy cannot undo")
	assert.NotContains(t, out, "started, having been")
}

// The two that can be started say which of them they were, because "stopped"
// and "interrupted" are the same state to the deploy and not to the reader.
func TestRender_AStartNamesWhatItIsStartingFrom(t *testing.T) {
	for status, want := range map[string]string{
		"stopped":     "started, having been stopped",
		"interrupted": "started, having been interrupted",
		"":            "started, having been stopped",
	} {
		out := render(t,
			Summary{Name: "my-app", WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0", Status: status},
			Plan{State: StateStopped})

		assert.Contains(t, out, want, "status %q", status)
	}
}

// The header carries the platform's own word rather than the state it reduces
// to. Stopped, suspended and interrupted are one state to the deploy and three
// different things to a reader, and only one of them can be started at all, so
// a suspended workload announced as "stopped" is a header that contradicts the
// refusal printed under it.
func TestRender_HeaderCarriesTheLiveStatusNotTheReducedState(t *testing.T) {
	out := render(t,
		Summary{Name: "my-app", WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0", Status: "suspended"},
		Plan{State: StateStopped})

	assert.Contains(t, out, "my-app (68b0c1d2), suspended")
	assert.NotContains(t, out, ", stopped")
}

// A summary with no status falls back to the state, which is what a plan built
// without a live workload has.
func TestRender_HeaderFallsBackToTheStateWithoutAStatus(t *testing.T) {
	out := render(t, Summary{Name: "my-app", WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0"}, Plan{State: StateStopped})

	assert.Contains(t, out, "my-app (68b0c1d2), stopped")
}

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

	assert.NotContains(t, out, "(", "there is no id to show yet")
	assert.Contains(t, out, "+ workload   my-app will be created, with its first artifact",
		"the create line carries the name, so nothing has to head the block")
	assert.Contains(t, out, "~ code       all project files, uploaded for the first time")
}

// A create that replaces a dead binding is not a first deploy and must not
// read as one. The plan is announced and then applied without a confirm, so
// this line is the entire warning a run pointed at the wrong instance gets.
func TestRender_RecreateNamesTheDeadBinding(t *testing.T) {
	out := render(t, Summary{Name: "my-app"}, Plan{
		State:           StateMissing,
		Creates:         true,
		PriorWorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0",
		Code:            CodeChange{Applies: true, FirstDeploy: true},
	})

	assert.Contains(t, out, "+ workload   my-app will be created: .datarobot.yaml is bound to 68b0c1d2, "+
		"which no longer exists")
	assert.NotContains(t, out, "with its first artifact",
		"this is a replacement for something that existed, not a first deploy")
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

	assert.Contains(t, out, "+ workload   will be created, with its first artifact")
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

// A locked artifact is only replaced by a run that mints a version. Saying so
// above a plan that changes the sizing in place, or one that only starts a
// stopped workload, describes a deploy that is not about to happen: both leave
// the locked artifact exactly where it is.
func TestRender_LockIsNotAnnouncedWhenNothingIsMinted(t *testing.T) {
	for _, tc := range []struct {
		name string
		plan Plan
	}{
		{"sizing alone", Plan{
			State:   StateRunning,
			Locked:  true,
			Runtime: []Change{{Path: "containerGroups[default].replicaCount", Have: 1.0, Want: 3.0}},
		}},
		{"a start", Plan{State: StateStopped, Locked: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotContains(t, render(t, appSummary, tc.plan), "lock")
		})
	}
}

// The other half: a run that does mint one has to say that the successor is
// locked too, because nothing in the file asked for that and it cannot be
// undone. A --yes run is never prompted, so the plan is the only warning.
func TestRender_LockIsAnnouncedWhenAVersionIsMinted(t *testing.T) {
	out := render(t, appSummary, Plan{
		State:  StateRunning,
		Locked: true,
		Code:   CodeChange{Applies: true, Files: 1},
	})

	assert.Contains(t, out, "~ lock")
	assert.Contains(t, out, "locked to match")
	assert.Contains(t, out, "permanent")
}

// A project whose link points at a locked artifact is redrafted onto a new one
// and the link moves, which the preview has to say: it is invisible, and a
// re-run does not undo it.
func TestRender_RedraftOfALockedLinkIsAnnounced(t *testing.T) {
	out := render(t, Summary{Name: "my-app"}, Plan{
		State:   StateUnbound,
		Creates: true,
		Code:    CodeChange{Applies: true, FirstDeploy: true, LinkLocked: true},
	})

	assert.Contains(t, out, "~ lock")
	assert.Contains(t, out, "pushes to that instead")
}

// A caller reading only the JSON has no other way to learn that this run will
// lock a version for good, or that it will redraft a locked link and move the
// project onto something else. The envelope's own Locked field is the live
// artifact's state, which is false on the create path precisely when the
// redraft is what is about to happen.
func TestPlanJSON_CarriesTheLockFacts(t *testing.T) {
	rolls := Plan{
		State:  StateRunning,
		Locked: true,
		Code:   CodeChange{Applies: true, Files: 1},
	}.JSON()

	assert.True(t, rolls.Locked)

	redrafts := Plan{
		State:   StateUnbound,
		Creates: true,
		Code:    CodeChange{Applies: true, FirstDeploy: true, LinkLocked: true},
	}.JSON()

	assert.True(t, redrafts.Code.LinkLocked)
	assert.False(t, redrafts.Locked, "no live artifact is being replaced on a create")
}

// The JSON is gated on the same question the printed plan is, or the two
// renderings of one plan would disagree about whether it locks anything.
func TestPlanJSON_NoLockFactsWhenNothingIsMinted(t *testing.T) {
	sizing := Plan{
		State:   StateRunning,
		Locked:  true,
		Runtime: []Change{{Path: "containerGroups[default].replicaCount", Have: 1.0, Want: 3.0}},
	}.JSON()

	assert.False(t, sizing.Locked, "a sizing change is applied in place and mints nothing")

	start := Plan{State: StateStopped, Locked: true, Code: CodeChange{Applies: true, LinkLocked: true}}.JSON()

	assert.False(t, start.Locked)
	assert.False(t, start.Code.LinkLocked)
}

// A project that already pushes to an artifact is not getting its first one.
// This is the flow a delete leaves behind: the binding is cleared and the
// artifact link is deliberately left exactly where it was.
func TestRender_CreateOnALinkedProjectDoesNotClaimAFirstArtifact(t *testing.T) {
	out := render(t, Summary{Name: "my-app"}, Plan{
		State:          StateUnbound,
		Creates:        true,
		LinkedArtifact: true,
		Code:           CodeChange{Applies: true, FirstDeploy: true},
	})

	assert.Contains(t, out, "on the artifact this project is linked to")
	assert.NotContains(t, out, "with its first artifact")
}

// The two facts are independent, and the flow a UI-side delete leaves behind
// produces both: the binding is stale and the artifact link is untouched. A
// line reporting only the dead binding drops the one thing that says which
// artifact the new workload comes up on.
func TestRender_RecreateOnALinkedProjectSaysBoth(t *testing.T) {
	out := render(t, Summary{Name: "my-app"}, Plan{
		State:           StateMissing,
		Creates:         true,
		PriorWorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0",
		LinkedArtifact:  true,
		Code:            CodeChange{Applies: true, FirstDeploy: true},
	})

	assert.Contains(t, out, "is bound to 68b0c1d2, which no longer exists")
	assert.Contains(t, out, "it comes up on the artifact this project is linked to")
}

// A published image consults no link, so a recreate says only what it knows.
func TestRender_RecreateOnAPublishedImageMentionsNoLink(t *testing.T) {
	out := render(t, Summary{Name: "my-app"}, Plan{
		State:           StateMissing,
		Creates:         true,
		PriorWorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0",
		LinkedArtifact:  true,
		Code:            CodeChange{Applies: false},
	})

	assert.Contains(t, out, "is bound to 68b0c1d2, which no longer exists")
	assert.NotContains(t, out, "linked to")
}

// A locked link is not reused: the run mints a replacement and moves the
// project onto it, which the lock line says in full. Claiming the create comes
// up on the linked artifact two lines above that is a plan contradicting
// itself, and the reader has no way to tell which half is true.
func TestRender_LockedLinkIsNotDescribedAsReused(t *testing.T) {
	out := render(t, Summary{Name: "my-app"}, Plan{
		State:          StateUnbound,
		Creates:        true,
		LinkedArtifact: true,
		Code:           CodeChange{Applies: true, FirstDeploy: true, LinkLocked: true},
	})

	assert.NotContains(t, out, "on the artifact this project is linked to")
	assert.NotContains(t, out, "with its first artifact",
		"a project that already pushes to an artifact is not getting its first")
	assert.Contains(t, out, "pushes to that instead", "the lock line is what explains this create")
}

// The same rule on the recreate line, which carries the dead binding first and
// the artifact fact second.
func TestRender_RecreateOnALockedLinkClaimsNoReuse(t *testing.T) {
	out := render(t, Summary{Name: "my-app"}, Plan{
		State:           StateMissing,
		Creates:         true,
		PriorWorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0",
		LinkedArtifact:  true,
		Code:            CodeChange{Applies: true, FirstDeploy: true, LinkLocked: true},
	})

	assert.Contains(t, out, "is bound to 68b0c1d2, which no longer exists")
	assert.NotContains(t, out, "it comes up on the artifact this project is linked to")
}

// A manifest naming a published image posts its artifact inline and never
// consults the link, so a linked project really is getting a new artifact.
// Claiming otherwise would describe a reuse that does not happen.
func TestRender_LinkedProjectOnAPublishedImageStillGetsANewArtifact(t *testing.T) {
	out := render(t, Summary{Name: "my-app"}, Plan{
		State:          StateUnbound,
		Creates:        true,
		LinkedArtifact: true,
		Code:           CodeChange{Applies: false},
	})

	assert.Contains(t, out, "with its first artifact")
	assert.NotContains(t, out, "linked to")
}
