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
	"fmt"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/workload/manifest"
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
		Artifact: []Change{moved(inPrimary("port"), 8080.0, 9090.0)},
		Runtime:  []Change{moved(inGroup("default", "replicaCount"), 1.0, 3.0)},
	}

	out := render(t, appSummary, plan)

	assert.Contains(t, out, "my-app (68b0c1d2), running")
	assert.Contains(t, out, "~ code       14 files changed since the last deploy")
	assert.Contains(t, out, "+ artifact   new version, 1 spec change")
	assert.Contains(t, out, "~ runtime    replicaCount: 1 -> 3")
	assert.NotContains(t, out, "Already up to date")
}

// TestRender_RefusedPlanIsDescribedRatherThanAnnounced is the reported bug:
// a state that cannot take a deploy still had its drift announced as work
// about to happen, and the reader only found out on the last line. The block
// survives, because knowing what differs is useful either way, but it is
// introduced as a description.
func TestRender_RefusedPlanIsDescribedRatherThanAnnounced(t *testing.T) {
	plan := Plan{
		State:    StateTerminated,
		Artifact: []Change{moved(inPrimary("port"), 8080.0, 9090.0)},
	}

	out := render(t, Summary{Name: "old-name", WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0", Refused: true}, plan)

	assert.Contains(t, out, "old-name (68b0c1d2), terminated")
	assert.Contains(t, out, "Nothing below will be applied")
	assert.Contains(t, out, "+ artifact   new version, 1 spec change",
		"the drift is still worth reading; only its framing changes")

	note := strings.Index(out, "Nothing below will be applied")
	change := strings.Index(out, "+ artifact")
	assert.Less(t, note, change,
		"the note has to precede the work it disclaims: below it is where the error already is")
}

// A plan that is going to be applied says nothing of the sort, which is the
// whole of what makes the note mean anything.
func TestRender_AppliedPlanCarriesNoRefusedNote(t *testing.T) {
	out := render(t, appSummary, Plan{State: StateRunning, Code: builtCode(3)})

	assert.NotContains(t, out, "Nothing below will be applied")
}

// TestRender_SingleRuntimeChangeSitsOnItsOwnLine: the common case is one
// number moving, and pushing it onto a sub-line for consistency's sake would
// cost a line to say nothing.
func TestRender_SingleRuntimeChangeSitsOnItsOwnLine(t *testing.T) {
	out := render(t, appSummary, Plan{
		State:   StateRunning,
		Runtime: []Change{moved(inGroup("default", "replicaCount"), 1.0, 3.0)},
	})

	assert.Contains(t, out, "~ runtime    replicaCount: 1 -> 3")
	assert.NotContains(t, out, "containerGroups", "one group is the answer for every line, so no line says it")
	assert.NotContains(t, out, "2 changes")
}

func TestRender_SeveralRuntimeChangesAreListed(t *testing.T) {
	out := render(t, appSummary, Plan{
		State: StateRunning,
		Runtime: []Change{
			moved(inGroup("default", "replicaCount"), 1.0, 3.0),
			moved(inPrimary("resourceAllocation", "cpu"), 0.5, 2.0),
		},
	})

	assert.Contains(t, out, "~ runtime    2 changes")

	// The group and the container it holds are two different places, so the
	// column that separates them earns its width here.
	assert.Contains(t, out, "      default  replicaCount: 1 -> 3")
	assert.Contains(t, out, "      primary  resourceAllocation.cpu: 0.5 -> 2")
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

	assert.Contains(t, out, "+ workload   my-app will be created: "+
		".datarobot.yaml is bound to 68b0c1d2e3f4a5b6c7d8e9f0, which no longer exists")
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
		Artifact: []Change{moved(inPrimary("imageUri"), "a:1", "a:2")},
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

// A deploy that keeps the running image takes seconds where one that rebuilds
// takes minutes, and --dry-run is the whole of the review a deploy gets.
func TestRender_ArtifactSaysWhetherTheImageIsKept(t *testing.T) {
	change := absent(inEnv("primary", "LOG_LEVEL"))

	kept := render(t, appSummary, Plan{State: StateRunning, InheritsImage: true, Artifact: []Change{change}})
	assert.Contains(t, kept, "new version, 1 spec change; keeps the running image, so no rebuild")

	building := render(t, appSummary, Plan{State: StateRunning, Artifact: []Change{change}})
	assert.Contains(t, building, "new version, 1 spec change")
	assert.NotContains(t, building, "no rebuild", "a deploy about to build says nothing about keeping an image")
}

// A draft takes the change itself, so the plan says the artifact is changed
// rather than that a new one is made, and the envelope says no version is
// minted: a lock fact gated on minting is not reported for it.
func TestRender_InPlaceSaysTheDraftIsWrittenTo(t *testing.T) {
	change := absent(inEnv("primary", "LOG_LEVEL"))
	plan := Plan{State: StateRunning, InheritsImage: true, InPlace: true, Artifact: []Change{change}}

	out := render(t, appSummary, plan)
	assert.Contains(t, out, "~ artifact   1 spec change, written to the draft in place; keeps the running image, so no rebuild")
	assert.NotContains(t, out, "new version")

	encoded := plan.JSON()
	assert.True(t, encoded.InPlace)
	assert.True(t, encoded.KeepsImage)
	assert.Equal(t, "rolled", encoded.Action)
	assert.False(t, plan.MintsVersion())

	again := render(t, appSummary, plan.rerolling("the last rollout of this version ended errored"))
	assert.Contains(t, again, "~ artifact   the last rollout of this version ended errored; rolling it again")
}

// An errored workload's line says what the deploy does about the failure,
// with the platform's reason beside the state, and the reason travels in the
// envelope too.
func TestRender_ErroredSaysWhetherTheDeployReplacesTheFailure(t *testing.T) {
	reason := "primary: ErrImagePull: failed to resolve image: not found"

	rolled := Plan{State: StateErrored, Reason: reason, Code: builtCode(0), ForceBuild: true}
	assert.Contains(t, render(t, appSummary, rolled),
		"~ workload   errored (primary: ErrImagePull: failed to resolve image: not found); this deploy replaces what it is running")
	assert.Contains(t, render(t, appSummary, rolled), "+ artifact   rebuilt from the synced code")
	assert.Equal(t, reason, rolled.JSON().StateReason)

	refused := render(t, appSummary, Plan{State: StateErrored, Reason: reason, Code: builtCode(0)})
	assert.Contains(t, refused,
		"! workload   errored (primary: ErrImagePull: failed to resolve image: not found), and nothing here would change what it runs")
	assert.NotContains(t, refused, "Already up to date")

	unexplained := render(t, appSummary, Plan{State: StateErrored, Code: builtCode(0)})
	assert.Contains(t, unexplained, "! workload   errored, and nothing here would change what it runs",
		"no reason is no parentheses, rather than empty ones")
}

// TestRender_NeverPrintsEnvironmentVariableValues is the one hard rule in
// here. A literal can be a secret someone pasted in plaintext, and a plan
// that echoed it would put it in scrollback and CI logs.
func TestRender_NeverPrintsEnvironmentVariableValues(t *testing.T) {
	const secret = "sk-abcdefghijklmnopqrstuvwxyz0123"

	newToken := absent(inEnv("primary", "NEW_TOKEN"))
	newToken.Want = map[string]any{"name": "NEW_TOKEN", "value": secret}

	plan := Plan{
		State: StateRunning,
		Artifact: []Change{
			moved(inEnv("primary", "OPENAI_API_KEY", "value"), "sk-oldvalue", secret),
			newToken,
		},
	}

	out := render(t, appSummary, plan)

	assert.NotContains(t, out, secret)
	assert.NotContains(t, out, "sk-oldvalue")
	assert.Contains(t, out, "env OPENAI_API_KEY: changed")
	assert.Contains(t, out, "env NEW_TOKEN: set")
	assert.Contains(t, out, "OPENAI_API_KEY", "the name is the whole point of showing the line")
}

// TestRender_CapsTheDetailListOutLoud: a truncated plan that did not say it
// was truncated would read as a complete account of what is about to happen.
func TestRender_CapsTheDetailListOutLoud(t *testing.T) {
	changes := make([]Change, 0, 10)
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		changes = append(changes, moved(Change{Path: "spec." + name, Keys: []string{"spec", name}}, 1.0, 2.0))
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
		Runtime: []Change{moved(inGroup("default", "replicaCount"), 1.0, 3.0)},
	})

	assert.Contains(t, out, "started")
	assert.Contains(t, out, "replicaCount: 1 -> 3")
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
		Artifact: []Change{moved(inPrimary("port"), 8080.0, 9090.0)},
		Runtime:  []Change{moved(inGroup("default", "replicaCount"), 1.0, 3.0)},
	}

	encoded, err := json.Marshal(plan.JSON())
	require.NoError(t, err)

	var decoded map[string]any

	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.Equal(t, "rolled", decoded["action"])
	assert.Equal(t, "running", decoded["state"])
	assert.Equal(t, false, decoded["creates"])
	assert.Empty(t, decoded["unbuildable"], "emitted even when empty, like reroll and stateReason")
	assert.Empty(t, decoded["incompatible"], "emitted even when empty, like unbuildable")

	code, _ := decoded["code"].(map[string]any)
	assert.Equal(t, true, code["changed"])
	assert.InDelta(t, 14.0, code["files"], 0)

	artifact, _ := decoded["artifact"].([]any)
	assert.Len(t, artifact, 1)
}

func TestPlanJSON_CarriesTheUnbuildableReason(t *testing.T) {
	plan := Plan{State: StateUnbound, Creates: true, Unbuildable: "the project has neither pyproject.toml with uv.lock nor package.json with package-lock.json"}

	encoded, err := json.Marshal(plan.JSON())
	require.NoError(t, err)

	var decoded map[string]any

	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.Equal(t, plan.Unbuildable, decoded["unbuildable"])
}

func TestPlanJSON_CarriesTheIncompatibleReason(t *testing.T) {
	plan := Plan{State: StateRunning, BoundArtifactID: "art-2", Incompatible: "artifact art-2 belongs to repository repo-2"}

	encoded, err := json.Marshal(plan.JSON())
	require.NoError(t, err)

	var decoded map[string]any

	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.Equal(t, plan.Incompatible, decoded["incompatible"])
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
		State:    StateRunning,
		Artifact: []Change{moved(inEnv("primary", "OPENAI_API_KEY", "value"), "old", secret)},
	}

	encoded, err := json.Marshal(plan.JSON())
	require.NoError(t, err)

	assert.NotContains(t, string(encoded), secret)
	assert.Contains(t, string(encoded), "OPENAI_API_KEY")

	// Whole, where the printed plan shortens it: a caller acting on the
	// envelope needs to know which container the variable is in, and it has no
	// column beside it to read that off.
	assert.Contains(t, string(encoded),
		"containerGroups[default].containers[primary].environmentVars[OPENAI_API_KEY].value: changed")
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
		Runtime: []Change{moved(inGroup("default", "replicaCount"), 1.0, 3.0)},
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

	assert.Contains(t, out, "is bound to 68b0c1d2e3f4a5b6c7d8e9f0, which no longer exists")
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

	assert.Contains(t, out, "is bound to 68b0c1d2e3f4a5b6c7d8e9f0, which no longer exists")
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

	assert.Contains(t, out, "is bound to 68b0c1d2e3f4a5b6c7d8e9f0, which no longer exists")
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

// A rotation moves nothing a plan can show, so an empty plan is the truth
// about the manifest and not about the workload: the run is about to restart
// it, and a bare verdict above that reads as a contradiction.
func TestRender_EmptyPlanSaysWhatTheRotationStillNeeds(t *testing.T) {
	out := &strings.Builder{}

	summary := Summary{Name: "my-app", WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0", Status: "running", SecretsRotated: 1}

	require.NoError(t, Render(out, summary, Plan{State: StateRunning}))

	assert.Contains(t, out.String(), "Already up to date")
	assert.Contains(t, out.String(), "Apart from the secret just re-sent")
}

// The line belongs to the rotation, not to every run that finds nothing to do.
func TestRender_EmptyPlanWithoutARotationSaysOnlyThat(t *testing.T) {
	out := &strings.Builder{}

	summary := Summary{Name: "my-app", WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0", Status: "running"}

	require.NoError(t, Render(out, summary, Plan{State: StateRunning}))

	assert.Contains(t, out.String(), "Already up to date")
	assert.NotContains(t, out.String(), "re-sent")
}

// moved is a change with its two values filled in. The fixtures build their
// paths through the same helpers the plan tests use, because the printed plan
// reads the walk's keys: a fixture carrying only a path would exercise a
// rendering nothing produces.
func moved(c Change, have, want any) Change {
	c.Have, c.Want = have, want

	return c
}

// inGroup is a change on a container group itself, which is where the sizing
// the runtime applies in place lives.
func inGroup(name string, field ...string) Change {
	return Change{
		Path: fmt.Sprintf("containerGroups[%s].%s", name, strings.Join(field, ".")),
		Keys: append([]string{keyContainerGroups, name}, field...),
	}
}

// inEnv is a change to one environment variable, with its path spelled the way
// the walk spells a name-keyed list. A change with no keys is redacted off
// that spelling, so a fixture that dotted it would be a shape the walk never
// produces standing in for the one it does.
func inEnv(container, name string, leaf ...string) Change {
	c := inContainer(container, append([]string{keyEnvironmentVars, name}, leaf...)...)

	c.Path = fmt.Sprintf("containerGroups[default].containers[%s].environmentVars[%s]", container, name)
	if len(leaf) > 0 {
		c.Path += "." + strings.Join(leaf, ".")
	}

	return c
}

// diffDriftPayload is the fixture the JSON diff tests read: drift in both
// halves (two spec leaves, one whole block the live object lacks, two sizing
// leaves) with unchanged and unmanaged fields around them, which is the least
// a section claiming to mirror the diff has to carry.
const diffDriftPayload = `{
  "name": "my-app",
  "artifact": {"name": "my-app-artifact", "spec": {
    "type": "service",
    "containerGroups": [{"name": "default", "containers": [
      {"name": "primary", "primary": true, "port": 9090,
       "readinessProbe": {"path": "/ready", "port": 8080},
       "startupProbe": {"path": "/started", "port": 8080}}
    ]}]
  }},
  "runtime": {"containerGroups": [
    {"name": "default", "replicaCount": 3,
     "containers": [{"name": "primary", "resourceAllocation": {"cpu": 1, "memory": "512MB"}}]}
  ]}
}`

// driftPlan builds that fixture the way a real run does, through Build, so
// the envelope and the diff read the same walk rather than two constructions.
func driftPlan(t *testing.T) Plan {
	t.Helper()

	plan, err := Build(
		loadedFrom(diffDriftPayload),
		liveFrom(t, StateRunning, planLiveSpec, planLiveRuntime),
		builtCode(0),
		Options{},
	)
	require.NoError(t, err)

	return plan
}

// TestPlanJSON_DiffAbsentWithoutTheFlag keeps the pre-feature envelope exact
// for a caller that never asked for a diff: no diff key at all, never a null
// one, on each of the three shapes an envelope can take.
func TestPlanJSON_DiffAbsentWithoutTheFlag(t *testing.T) {
	cases := []struct {
		name string
		plan Plan
	}{
		{"nothing changed", Plan{State: StateRunning, Code: builtCode(0)}},
		{"a sizing change", Plan{
			State:   StateRunning,
			Runtime: []Change{{Path: "containerGroups[default].replicaCount", Have: 1.0, Want: 3.0}},
		}},
		{"a first deploy", Plan{State: StateUnbound, Creates: true}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			encoded, err := json.Marshal(c.plan.JSON())
			require.NoError(t, err)

			var decoded map[string]any

			require.NoError(t, json.Unmarshal(encoded, &decoded))
			assert.NotContains(t, decoded, "diff",
				"the section is added only when --diff asked for it, not nulled out")
		})
	}
}

// TestPlanJSON_DiffCarriesTheChangingLeaves: with --diff the plan carries one
// entry per changing leaf, in the walk's order, with the unchanged context
// leaves left out of it and the unmanaged paths listed beside them. The
// legacy artifact/runtime arrays ride along unchanged, because --diff adds a
// section rather than reshaping the envelope a consumer already reads.
func TestPlanJSON_DiffCarriesTheChangingLeaves(t *testing.T) {
	plan := driftPlan(t)

	encoded, err := json.Marshal(plan.JSONWithDiff())
	require.NoError(t, err)

	var decoded map[string]any

	require.NoError(t, json.Unmarshal(encoded, &decoded))

	diff, ok := decoded["diff"].(map[string]any)
	require.True(t, ok, "--diff was requested, so the section is an object")

	changes, ok := diff["changes"].([]any)
	require.True(t, ok, "changes is an array even when it is empty")

	got := make([]string, 0, len(changes))

	for _, c := range changes {
		entry, ok := c.(map[string]any)
		require.True(t, ok)

		path, _ := entry["path"].(string)
		got = append(got, path)

		assert.Contains(t, entry, "have")
		assert.Contains(t, entry, "want")
		assert.Contains(t, entry, "absent")
		assert.NotContains(t, entry, "redacted", "nothing on this fixture is an environment variable")
	}

	// One entry per changing leaf of the manifest, in the same order the
	// walk reports them to the default envelope: the two halves concatenated,
	// unchanged-but-managed leaves nowhere among them, and the sidecar the
	// file never names last in each half, because this plan rolls and
	// resizes and so drops it from both.
	want := make([]string, 0, len(plan.Artifact)+len(plan.Runtime)+2)

	want = append(want, paths(plan.Artifact)...)
	want = append(want, "containerGroups[default].containers[metrics]")
	want = append(want, paths(plan.Runtime)...)
	want = append(want, "containerGroups[default].containers[metrics]")

	assert.Equal(t, want, got)
	assert.Len(t, got, 7, "five drifting leaves across the two halves, plus the sidecar dropped from each")
	assert.Contains(t, got, "containerGroups[default].containers[primary].startupProbe")
	assert.NotContains(t, got, "containerGroups[default].containers[primary].port.expectation",
		"the walk only answers for leaves the file names")

	// The absent leaf carries no live value, and says so twice: a null have
	// and an absent flag, because "not there" is a different act from
	// "different value".
	var added map[string]any

	for _, c := range changes {
		entry, _ := c.(map[string]any)

		if entry["path"] == "containerGroups[default].containers[primary].startupProbe" {
			added = entry

			break
		}
	}

	require.NotNil(t, added)
	assert.Nil(t, added["have"])
	assert.Equal(t, true, added["absent"])
	assert.NotNil(t, added["want"])

	// The dropped sidecar is a removal: the live element as have, no want,
	// and the marker that says so.
	var removed []map[string]any

	for _, c := range changes {
		entry, _ := c.(map[string]any)

		if entry["removed"] == true {
			removed = append(removed, entry)
		}
	}

	require.Len(t, removed, 2, "the sidecar is dropped from the spec and from the runtime")

	for _, entry := range removed {
		assert.Equal(t, "containerGroups[default].containers[metrics]", entry["path"])
		assert.NotNil(t, entry["have"])
		assert.Nil(t, entry["want"])
	}

	// Nothing is left alone by a plan that rewrites both halves.
	unmanaged, ok := diff["unmanaged"].([]any)
	require.True(t, ok, "unmanaged is an array even when it is empty")
	assert.Empty(t, unmanaged)

	// The arrays a consumer already reads keep their exact content with and
	// without the section.
	plain := plan.JSON()

	assert.Equal(t, plain.Artifact, plan.JSONWithDiff().Artifact)
	assert.Equal(t, plain.Runtime, plan.JSONWithDiff().Runtime)
}

// TestPlanJSON_DiffRedactsBeforeSerialising is the JSON half of the one hard
// rule. A literal can be a secret pasted in plaintext and a credential ref
// names one; both ride in the diff rows as raw values, so the values are
// withheld here rather than trusted to a marshalling order, and the entry
// carries a marker saying the refusal happened.
//
// The sidecar container pins the whole-element half of the rule: a NEW
// name-keyed list element is one row for the element, not one per field, so
// the row's own path names the container and never trips the path-based
// redaction while its value carries an environmentVars block complete with
// secrets. Nothing a variable holds may reach the document.
func TestPlanJSON_DiffRedactsBeforeSerialising(t *testing.T) {
	const (
		literal       = "sk-literal-plaintext"
		oldLit        = "sk-old-plaintext"
		rotated       = "aaaa222222222222222222aa"
		liveID        = "66f1a2b3c4d5e6f7a8b9c0d1"
		sidecarLit    = "hunter2-plaintext"
		sidecarCredID = "77c2b3c4d5e6f7a8b9c0d199"
	)

	payload := `{
	  "name": "my-app",
	  "artifact": {"name": "my-app-artifact", "spec": {
	    "type": "service",
	    "containerGroups": [{"name": "default", "containers": [
	      {"name": "primary", "environmentVars": [
	        {"name": "OPENAI_API_KEY", "value": "` + literal + `"},
	        {"name": "HUGGING_FACE_HUB_TOKEN", "source": "dr-credential",
	         "drCredentialId": "` + rotated + `", "key": "apiToken"}
	      ]},
	      {"name": "sidecar", "image": "sidecar:2", "environmentVars": [
	        {"name": "SIDECAR_PASSWORD", "value": "` + sidecarLit + `"},
	        {"name": "SIDECAR_API_TOKEN", "source": "dr-credential",
	         "drCredentialId": "` + sidecarCredID + `", "key": "sidecarKey"}
	      ]}
	    ]}]
	  }}
	}`

	live := `{
	  "type": "service",
	  "containerGroups": [{"name": "default", "containers": [
	    {"name": "primary", "environmentVars": [
	      {"name": "OPENAI_API_KEY", "value": "` + oldLit + `"},
	      {"name": "HUGGING_FACE_HUB_TOKEN", "source": "dr-credential",
	       "drCredentialId": "` + liveID + `", "key": "apiToken"}
	    ]}
	  ]}]
	}`

	plan, err := Build(loadedFrom(payload), liveFrom(t, StateRunning, live, planLiveRuntime), builtCode(0), Options{})
	require.NoError(t, err)

	// The fixture has to earn its keep: two env-var changes on the existing
	// container plus the whole-element row the new container emits, and all
	// three are what must not leak.
	require.Equal(t, []string{
		"containerGroups[default].containers[primary].environmentVars[OPENAI_API_KEY].value",
		"containerGroups[default].containers[primary].environmentVars[HUGGING_FACE_HUB_TOKEN].drCredentialId",
		"containerGroups[default].containers[sidecar]",
	}, paths(plan.Artifact))

	encoded, err := json.Marshal(plan.JSONWithDiff())
	require.NoError(t, err)

	document := string(encoded)

	for _, secret := range []string{literal, oldLit, rotated, liveID, sidecarLit, sidecarCredID, "dr-credential:"} {
		assert.NotContains(t, document, secret)
	}

	var decoded map[string]any

	require.NoError(t, json.Unmarshal(encoded, &decoded))

	diff := decoded["diff"].(map[string]any)
	changes := diff["changes"].([]any)
	require.Len(t, changes, 3, "the file names no runtime, so the roll drops nothing from it")

	for _, c := range changes {
		entry := c.(map[string]any)

		if entry["path"] == "containerGroups[default].containers[sidecar]" {
			continue
		}

		assert.Contains(t, entry["path"], "environmentVars[", "the name is the whole point of the entry")
		assert.Nil(t, entry["have"], "the live value never serialises")
		assert.Nil(t, entry["want"], "the asked-for value never serialises")
		assert.Equal(t, true, entry["redacted"])
	}

	// The new container's entry arrives as a whole-element row: the path
	// names the container, so the scrub works on the subtree instead. The
	// names of its variables stay, because seeing which variables an element
	// sets is half the point of the entry; every value, literal or
	// credential ref, is gone, and the refusal is marked.
	var sidecar map[string]any

	for _, c := range changes {
		entry, _ := c.(map[string]any)

		if entry["path"] == "containerGroups[default].containers[sidecar]" {
			sidecar = entry

			break
		}
	}

	require.NotNil(t, sidecar, "the new container is one whole-element change")
	assert.Equal(t, true, sidecar["redacted"])
	assert.Nil(t, sidecar["have"], "the container is an addition")
	assert.Equal(t, true, sidecar["absent"])

	want, ok := sidecar["want"].(map[string]any)
	require.True(t, ok, "the container's other fields keep their structure")

	assert.Equal(t, "sidecar:2", want["image"], "nothing secret keeps the structure honest")

	vars, ok := want["environmentVars"].([]any)
	require.True(t, ok, "the variable names stay")
	require.Len(t, vars, 2)

	assert.Equal(t, map[string]any{"name": "SIDECAR_PASSWORD"}, vars[0])
	assert.Equal(t, map[string]any{"name": "SIDECAR_API_TOKEN"}, vars[1])

	// The legacy arrays were redacted before this section existed, and stay
	// that way: they say the change happened, never what it said.
	plain := plan.JSON()

	assert.NotContains(t, strings.Join(plain.Artifact, "\n"), literal)
	assert.Contains(t, strings.Join(plain.Artifact, "\n"), "changed")
	assert.Equal(t, plain.Artifact, plan.JSONWithDiff().Artifact)
}

// TestPlanJSON_DiffRedactsAWholeEnvVarsList: when the live side carries no
// environmentVars list at all, the walker has nothing to match variables
// against by name, so the file's whole list arrives as ONE row whose path
// ENDS at the list. That row is as much a carrier of variable values as any
// whole-element row, and the document must scrub it the same way: the names
// stay, every value -- literal or credential ref -- is gone, and the refusal
// is marked. (Caught against live staging: the container-level scrub only
// saw values nested BELOW a row, never a row that was the list itself.)
func TestPlanJSON_DiffRedactsAWholeEnvVarsList(t *testing.T) {
	const (
		literal = "sk-plaintext-9999"
		credID  = "88d3c4d5e6f7a8b9c0d1e2f3"
	)

	payload := `{
	  "name": "my-app",
	  "artifact": {"name": "my-app-artifact", "spec": {
	    "type": "service",
	    "containerGroups": [{"name": "default", "containers": [
	      {"name": "primary", "environmentVars": [
	        {"name": "OPENAI_API_KEY", "value": "` + literal + `"},
	        {"name": "HUGGING_FACE_HUB_TOKEN", "source": "dr-credential",
	         "drCredentialId": "` + credID + `", "key": "apiToken"}
	      ]}
	    ]}]
	  }}
	}`

	// The live container exists but names no variables, so there is no list
	// to walk element-by-element: the file's whole environmentVars list is
	// one changed leaf.
	live := `{
	  "type": "service",
	  "containerGroups": [{"name": "default", "containers": [
	    {"name": "primary", "image": "app:1"}
	  ]}]
	}`

	plan, err := Build(loadedFrom(payload), liveFrom(t, StateRunning, live, planLiveRuntime), builtCode(0), Options{})
	require.NoError(t, err)

	// The fixture has to earn its keep: the change must arrive as the
	// whole-list row, not as per-variable rows.
	assert.Contains(t, paths(plan.Artifact),
		"containerGroups[default].containers[primary].environmentVars",
		"expected the whole environmentVars list as one changed leaf")

	encoded, err := json.Marshal(plan.JSONWithDiff())
	require.NoError(t, err)

	document := string(encoded)

	for _, secret := range []string{literal, credID, "dr-credential:"} {
		assert.NotContains(t, document, secret)
	}

	var decoded map[string]any

	require.NoError(t, json.Unmarshal(encoded, &decoded))

	changes := decoded["diff"].(map[string]any)["changes"].([]any)

	var entry map[string]any

	for _, c := range changes {
		e, _ := c.(map[string]any)

		if e["path"] == "containerGroups[default].containers[primary].environmentVars" {
			entry = e

			break
		}
	}

	require.NotNil(t, entry, "the whole-list change must survive the scrub")
	assert.Equal(t, true, entry["redacted"])

	want, ok := entry["want"].([]any)
	require.True(t, ok, "the variable names keep their list")

	assert.Equal(t, []any{
		map[string]any{"name": "OPENAI_API_KEY"},
		map[string]any{"name": "HUGGING_FACE_HUB_TOKEN"},
	}, want, "only the names survive the scrub")
}

// TestPlanJSON_FirstDeployDiffIsAllAdditions: with no live object every leaf
// of the compiled manifest is an addition, absent with no live value, and
// there is nothing for the unmanaged list to hold.
func TestPlanJSON_FirstDeployDiffIsAllAdditions(t *testing.T) {
	plan, err := Build(
		loadedFrom(planPayload),
		Live{State: StateUnbound},
		CodeChange{Applies: true, FirstDeploy: true},
		Options{},
	)
	require.NoError(t, err)

	encoded, err := json.Marshal(plan.JSONWithDiff())
	require.NoError(t, err)

	var decoded map[string]any

	require.NoError(t, json.Unmarshal(encoded, &decoded))

	assert.Equal(t, true, decoded["creates"])
	assert.Empty(t, decoded["priorWorkloadId"])

	diff, ok := decoded["diff"].(map[string]any)
	require.True(t, ok)

	changes := diff["changes"].([]any)
	require.NotEmpty(t, changes, "a first deploy is made of the leaves the manifest names")

	for _, c := range changes {
		entry := c.(map[string]any)

		assert.Equal(t, true, entry["absent"], "%v must be an addition", entry["path"])
		assert.Nil(t, entry["have"])
		assert.NotNil(t, entry["want"])
	}

	unmanaged := diff["unmanaged"].([]any)
	assert.Empty(t, unmanaged, "there is no live object to carry unmanaged fields")

	plain := plan.JSON()

	assert.Equal(t, plain.Artifact, plan.JSONWithDiff().Artifact)
	assert.Equal(t, plain.Runtime, plan.JSONWithDiff().Runtime)
}

// TestPlanJSON_EmptyPlanDiffIsTwoEmptyLists: --diff still asked, so the
// section is present with empty arrays where a change would sit, and the
// rest of the envelope is the exact document the default path would have
// produced.
func TestPlanJSON_EmptyPlanDiffIsTwoEmptyLists(t *testing.T) {
	plan := Plan{State: StateRunning, Code: builtCode(0)}

	encoded, err := json.Marshal(plan.JSONWithDiff())
	require.NoError(t, err)

	var withDiff map[string]any

	require.NoError(t, json.Unmarshal(encoded, &withDiff))

	diff, ok := withDiff["diff"].(map[string]any)
	require.True(t, ok, "the section is present because --diff asked, even for an empty plan")

	changes, ok := diff["changes"].([]any)
	require.True(t, ok, "changes is [] rather than null")
	assert.Empty(t, changes)

	unmanaged, ok := diff["unmanaged"].([]any)
	require.True(t, ok, "unmanaged is [] rather than null")
	assert.Empty(t, unmanaged)

	plainEncoded, err := json.Marshal(plan.JSON())
	require.NoError(t, err)

	var plain map[string]any

	require.NoError(t, json.Unmarshal(plainEncoded, &plain))

	delete(withDiff, "diff")
	assert.Equal(t, plain, withDiff, "aside from the section, the two envelopes are one document")
}

// textDiffChangePaths pulls the paths of a rendered diff's change lines, in
// order. A changed leaf states both sides of itself, `- old` then `+ new`,
// and is one entry in the JSON list, so a run of same-path lines collapses to
// one. The text and the list then say the same sequence, not just the same set.
func textDiffChangePaths(t *testing.T, body string) []string {
	t.Helper()

	var out []string

	for _, line := range strings.Split(body, "\n") {
		if len(line) < 2 || (line[0] != '+' && line[0] != '-') {
			continue
		}

		path, _, _ := strings.Cut(line[2:], ": ")

		if len(out) > 0 && out[len(out)-1] == path {
			continue
		}

		out = append(out, path)
	}

	return out
}

// TestPlanJSON_DiffOrderMatchesTheTextBody: both renderings draw from the one
// walk the plan was built with, so the Nth entry of the list is the Nth
// change the text prints, artifact side then runtime side, synthetic
// artifact-id change included. A consumer reading both representations
// deserves a sequence it can zip, and a list that reordered itself between
// the two would make every such zip a lie.
func TestPlanJSON_DiffOrderMatchesTheTextBody(t *testing.T) {
	plan := driftPlan(t)

	var body strings.Builder

	require.NoError(t, RenderDiff(&body, appSummary, plan))

	encoded, err := json.Marshal(plan.JSONWithDiff())
	require.NoError(t, err)

	var decoded PlanJSON

	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.NotNil(t, decoded.Diff)
	require.NotEmpty(t, decoded.Diff.Changes, "the fixture drifts, so both renderings have changes to line up")

	fromJSON := make([]string, 0, len(decoded.Diff.Changes))

	for _, c := range decoded.Diff.Changes {
		fromJSON = append(fromJSON, c.Path)
	}

	assert.Equal(t, textDiffChangePaths(t, body.String()), fromJSON)
}

// TestPlanJSON_DiffSyntheticChangesSurvive: the artifact id is a change no
// walk of the spec can produce, because a file bound by id describes no spec
// at all. The default envelope reports it, so the diff section must too --
// a version roll invisible in one rendering of the plan it drives is a
// silent regression waiting for a reader.
func TestPlanJSON_DiffSyntheticChangesSurvive(t *testing.T) {
	loaded := Loaded{Compiled: &manifest.Compiled{
		Payload:    json.RawMessage(`{"name": "my-app", "artifactId": "68b0bbbb0000000000000002"}`),
		ArtifactID: "68b0bbbb0000000000000002",
	}}

	live := liveFrom(t, StateRunning, "", planLiveRuntime)
	live.ArtifactID = "68a0000000000000000000a1"

	plan, err := Build(loaded, live, builtCode(0), Options{})
	require.NoError(t, err)

	require.Equal(t, []string{"artifactId"}, paths(plan.Artifact),
		"the fixture has to reach Build's synthetic append for the test to mean anything")

	encoded, err := json.Marshal(plan.JSONWithDiff())
	require.NoError(t, err)

	var decoded PlanJSON

	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.NotNil(t, decoded.Diff)

	require.Len(t, decoded.Diff.Changes, 1)

	change := decoded.Diff.Changes[0]

	assert.Equal(t, "artifactId", change.Path)
	assert.Equal(t, "68a0000000000000000000a1", change.Have)
	assert.Equal(t, "68b0bbbb0000000000000002", change.Want)
	assert.False(t, change.Absent)
}

// The type is the same kind of blind spot: the platform reads the
// discriminator off the artifact, so the spec walk never sees it. It rides in
// the synthetic append beside the id, and lands in the list the same way.
func TestPlanJSON_DiffSyntheticTypeChangeSurvives(t *testing.T) {
	loaded := Loaded{Compiled: &manifest.Compiled{
		Payload: json.RawMessage(`{
		  "name": "my-app",
		  "artifact": {"name": "my-app-artifact", "type": "agent", "spec": {}}
		}`),
	}}

	live := liveFrom(t, StateRunning, "", planLiveRuntime)
	live.ArtifactType = "service"

	plan, err := Build(loaded, live, builtCode(0), Options{})
	require.NoError(t, err)

	require.Equal(t, []string{"artifact.type"}, paths(plan.Artifact))

	encoded, err := json.Marshal(plan.JSONWithDiff())
	require.NoError(t, err)

	var decoded PlanJSON

	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.NotNil(t, decoded.Diff)
	require.Len(t, decoded.Diff.Changes, 1)

	change := decoded.Diff.Changes[0]

	assert.Equal(t, "artifact.type", change.Path)
	assert.Equal(t, "service", change.Have)
	assert.Equal(t, "agent", change.Want)
}
