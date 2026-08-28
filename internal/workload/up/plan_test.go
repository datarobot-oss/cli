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

// loadedFrom builds a Loaded around a compiled payload. The parse tree is nil
// on purpose: Build reads the file through the payload alone.
func loadedFrom(payload string) Loaded {
	return Loaded{Compiled: &manifest.Compiled{Payload: json.RawMessage(payload)}}
}

// liveFrom builds the live side from the two cleaned documents.
func liveFrom(t *testing.T, state State, specJSON, runtimeJSON string) Live {
	t.Helper()

	live := Live{State: state}

	if specJSON != "" {
		live.Spec = obj(t, specJSON)
	}

	if runtimeJSON != "" {
		live.Runtime = obj(t, runtimeJSON)
	}

	return live
}

// builtCode is a code answer for a project whose image the platform builds.
func builtCode(files int) CodeChange {
	return CodeChange{Applies: true, Files: files}
}

func TestCodeChange_Changed(t *testing.T) {
	cases := []struct {
		name string
		code CodeChange
		want bool
	}{
		{"a published image has no code to sync", CodeChange{Applies: false}, false},
		{"a published image is unaffected by a dirty tree", CodeChange{Applies: false, Files: 9}, false},
		{"nothing differs", builtCode(0), false},
		{"files differ", builtCode(3), true},
		{"a first deploy has everything to send", CodeChange{Applies: true, FirstDeploy: true}, true},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, c.code.Changed(), c.name)
	}
}

// TestPlan_EmptyIsExactlyNothingToDo guards the "Already up to date" exit.
// Reporting empty when something did differ skips a deploy the user asked
// for; reporting non-empty when nothing differs rebuilds and rolls a running
// workload for no reason.
func TestPlan_EmptyIsExactlyNothingToDo(t *testing.T) {
	cases := []struct {
		name string
		plan Plan
		want bool
	}{
		{"running and identical", Plan{State: StateRunning, Code: builtCode(0)}, true},
		{"a published image with a dirty tree", Plan{State: StateRunning, Code: CodeChange{Files: 5}}, true},
		{"code differs", Plan{State: StateRunning, Code: builtCode(1)}, false},
		{"spec differs", Plan{State: StateRunning, Artifact: []Change{{Path: "port"}}}, false},
		{"sizing differs", Plan{State: StateRunning, Runtime: []Change{{Path: "replicaCount"}}}, false},
		{"nothing exists yet", Plan{State: StateUnbound, Creates: true}, false},
		{"stopped and identical", Plan{State: StateStopped, Code: builtCode(0)}, false},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, c.plan.Empty(), c.name)
	}
}

// TestPlan_StoppedIsNeverEmpty states the reasoning outright: the user asked
// to deploy, and a workload that is not running has not been deployed, so
// matching the file is not enough to call the job done.
func TestPlan_StoppedIsNeverEmpty(t *testing.T) {
	plan := Plan{State: StateStopped, Code: builtCode(0)}

	assert.False(t, plan.Empty())
	assert.Equal(t, ActionStarted, plan.Action())
}

func TestPlan_RollsArtifact(t *testing.T) {
	cases := []struct {
		name string
		plan Plan
		want bool
	}{
		{"code differs", Plan{Code: builtCode(2)}, true},
		{"spec differs", Plan{Artifact: []Change{{Path: "port"}}}, true},
		{"sizing alone never mints a version", Plan{Runtime: []Change{{Path: "replicaCount"}}}, false},
		{"a creation is not a roll", Plan{Creates: true, Code: builtCode(2)}, false},
		{
			// A rendered path would cut at the name's bracket and read "b]".
			"a container name holding a bracket cannot steer the answer",
			Plan{Artifact: []Change{{
				Path: "containerGroups[default].containers[a]b].imageUri",
				Keys: []string{keyContainerGroups, "default", keyContainers, "a]b", "imageUri"},
			}}},
			true,
		},
		{"nothing differs", Plan{Code: builtCode(0)}, false},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, c.plan.RollsArtifact(), c.name)
	}
}

func TestPlan_RebuildsImage(t *testing.T) {
	// A container name holding a bracket: a rendered path would cut at it and
	// read "b]" as the field, which is in no allow-list.
	bracketed := Change{
		Path: "containerGroups[default].containers[a]b].imageUri",
		Keys: []string{keyContainerGroups, "default", keyContainers, "a]b", "imageUri"},
	}

	cases := []struct {
		name string
		plan Plan
		want bool
	}{
		{"changed code is a new image", Plan{Code: builtCode(2)}, true},
		{"nothing differs", Plan{Code: builtCode(0)}, false},
		{
			"an environment variable is read at start",
			Plan{Artifact: []Change{inPrimary("environmentVars", "LOG_LEVEL", "value")}},
			false,
		},
		// The wizard's agent card flag sits under the spec, not a container.
		{
			"a spec-level field the CLI writes is read at start too",
			Plan{Artifact: []Change{{Path: "a2aEnabled", Keys: []string{"a2aEnabled"}}}},
			false,
		},
		{
			"the dockerfile is what a build reads",
			Plan{Artifact: []Change{inPrimary("imageBuildConfig", "dockerfile", "source")}},
			true,
		},
		{"and the image itself", Plan{Artifact: []Change{inPrimary("imageUri")}}, true},
		{
			"a field this release has never heard of is a build input until it is known not to be",
			Plan{Artifact: []Change{inPrimary("buildArgs", "VERSION")}},
			true,
		},
		{
			"a whole container added is not a change to how one runs",
			Plan{Artifact: []Change{absent(inContainer("sidecar"))}},
			true,
		},
		{
			"a change of kind starts a lineage with nothing to inherit from",
			Plan{Artifact: []Change{{Path: keyArtifactType, Keys: []string{keyArtifactType}}}},
			true,
		},
		{
			"a container name holding a bracket cannot steer the answer",
			Plan{Artifact: []Change{bracketed}},
			true,
		},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, c.plan.RebuildsImage(), c.name)
	}
}

func inPrimary(field ...string) Change {
	return inContainer("primary", field...)
}

func inContainer(name string, field ...string) Change {
	keys := append([]string{keyContainerGroups, "default", keyContainers, name}, field...)

	path := fmt.Sprintf("containerGroups[default].containers[%s]", name)
	if len(field) > 0 {
		path += "." + strings.Join(field, ".")
	}

	return Change{Path: path, Keys: keys}
}

func absent(change Change) Change {
	change.Absent = true

	return change
}

// TestPlan_ActionPicksTheMostSignificant pins the priority order that the
// JSON envelope reports. A run does one thing, so when several apply the
// headline has to be the one that changed the most.
func TestPlan_ActionPicksTheMostSignificant(t *testing.T) {
	cases := []struct {
		name string
		plan Plan
		want string
	}{
		{"nothing", Plan{State: StateRunning}, ActionUnchanged},
		{"first deploy", Plan{State: StateUnbound, Creates: true}, ActionCreated},
		{"creating outranks everything", Plan{
			State: StateUnbound, Creates: true, Code: builtCode(4),
			Artifact: []Change{{Path: "port"}}, Runtime: []Change{{Path: "replicaCount"}},
		}, ActionCreated},
		{"code", Plan{State: StateRunning, Code: builtCode(4)}, ActionRolled},
		{"spec", Plan{State: StateRunning, Artifact: []Change{{Path: "port"}}}, ActionRolled},
		{"rolling outranks retuning", Plan{
			State: StateRunning, Code: builtCode(4), Runtime: []Change{{Path: "replicaCount"}},
		}, ActionRolled},
		{"sizing only", Plan{State: StateRunning, Runtime: []Change{{Path: "replicaCount"}}}, ActionUpdated},
		{"retuning outranks starting", Plan{
			State: StateStopped, Runtime: []Change{{Path: "replicaCount"}},
		}, ActionUpdated},
		{"stopped with nothing to change", Plan{State: StateStopped}, ActionStarted},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, c.plan.Action(), c.name)
	}
}

const planPayload = `{
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
      {"name": "default", "replicaCount": 1,
       "containers": [{"name": "primary", "resourceAllocation": {"cpu": 0.5, "memory": "512MB"}}]}
    ]
  }
}`

const planLiveSpec = `{
  "type": "service",
  "containerGroups": [
    {"name": "default", "containers": [
      {"name": "primary", "primary": true, "port": 8080,
       "readinessProbe": {"path": "/health", "port": 8080}},
      {"name": "metrics", "imageUri": "registry/metrics:v1"}
    ]}
  ]
}`

const planLiveRuntime = `{
  "containerGroups": [
    {"name": "default", "replicaCount": 1, "resourceBundles": ["cpu.small"],
     "containers": [
       {"name": "primary", "resourceAllocation": {"cpu": 0.5, "memory": "512MB"}},
       {"name": "metrics", "resourceAllocation": {"cpu": 0.1, "memory": "128MB"}}
     ]}
  ]
}`

// TestBuild_MatchingLiveStateIsEmpty runs a manifest that says less than the
// workload against that workload. The sidecar and the resource bundle are
// unmentioned, so neither is drift, and the run is a no-op.
func TestBuild_MatchingLiveStateIsEmpty(t *testing.T) {
	plan, err := Build(
		loadedFrom(planPayload),
		liveFrom(t, StateRunning, planLiveSpec, planLiveRuntime),
		builtCode(0),
		Options{},
	)
	require.NoError(t, err)

	assert.True(t, plan.Empty())
	assert.Equal(t, ActionUnchanged, plan.Action())
	assert.Empty(t, plan.Artifact)
	assert.Empty(t, plan.Runtime)
}

func TestBuild_SplitsDriftIntoItsTwoHalves(t *testing.T) {
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

	assert.Equal(t,
		[]string{"containerGroups[default].containers[primary].port"},
		paths(plan.Artifact))
	assert.Equal(t,
		[]string{"containerGroups[default].replicaCount"},
		paths(plan.Runtime))
	assert.Equal(t, ActionRolled, plan.Action(), "a spec change mints a version even with sizing alongside")
}

// A file bound to an artifact by id describes no spec, so the field walk has
// nothing to compare. Pointing at a version other than the one running is
// still the entire plan, and reporting "already up to date" while the file
// and the workload disagree would be the worst kind of wrong.
func TestBuild_BoundToAnotherArtifactIsARoll(t *testing.T) {
	loaded := Loaded{Compiled: &manifest.Compiled{
		Payload:    json.RawMessage(`{"name": "my-app", "artifactId": "68b0bbbb0000000000000002"}`),
		ArtifactID: "68b0bbbb0000000000000002",
	}}

	live := liveFrom(t, StateRunning, "", planLiveRuntime)
	live.ArtifactID = "68a0000000000000000000a1"

	plan, err := Build(loaded, live, builtCode(0), Options{})
	require.NoError(t, err)

	assert.Equal(t, []string{"artifactId"}, paths(plan.Artifact))
	assert.Equal(t, ActionRolled, plan.Action())
	assert.False(t, plan.Empty())
}

// The same file against the workload already running that artifact plans
// nothing, or every deploy would roll a version onto itself.
func TestBuild_BoundToTheRunningArtifactIsEmpty(t *testing.T) {
	loaded := Loaded{Compiled: &manifest.Compiled{
		Payload:    json.RawMessage(`{"name": "my-app", "artifactId": "68a0000000000000000000a1"}`),
		ArtifactID: "68a0000000000000000000a1",
	}}

	live := liveFrom(t, StateRunning, "", planLiveRuntime)
	live.ArtifactID = "68a0000000000000000000a1"

	plan, err := Build(loaded, live, builtCode(0), Options{})
	require.NoError(t, err)

	assert.Empty(t, plan.Artifact)
	assert.True(t, plan.Empty())
}

func TestBuild_RuntimeOnlyDriftDoesNotRoll(t *testing.T) {
	resized := `{
	  "name": "my-app",
	  "artifact": {"name": "my-app-artifact", "spec": {
	    "type": "service",
	    "containerGroups": [{"name": "default", "containers": [
	      {"name": "primary", "primary": true, "port": 8080,
	       "readinessProbe": {"path": "/health", "port": 8080}}
	    ]}]
	  }},
	  "runtime": {"containerGroups": [
	    {"name": "default", "replicaCount": 3,
	     "containers": [{"name": "primary", "resourceAllocation": {"cpu": 0.5, "memory": "512MB"}}]}
	  ]}
	}`

	plan, err := Build(
		loadedFrom(resized),
		liveFrom(t, StateRunning, planLiveSpec, planLiveRuntime),
		builtCode(0),
		Options{},
	)
	require.NoError(t, err)

	assert.False(t, plan.RollsArtifact(), "sizing is tunable without a new artifact version")
	assert.Equal(t, ActionUpdated, plan.Action())
}

// TestBuild_UnboundDoesNotDiff: with nothing to compare against, every field
// is trivially an addition. Saying that once is a plan; saying it per field
// is a wall of text that hides what the run will actually do.
func TestBuild_UnboundDoesNotDiff(t *testing.T) {
	plan, err := Build(
		loadedFrom(planPayload),
		Live{State: StateUnbound},
		CodeChange{Applies: true, FirstDeploy: true},
		Options{},
	)
	require.NoError(t, err)

	assert.True(t, plan.Creates)
	assert.Empty(t, plan.Artifact)
	assert.Empty(t, plan.Runtime)
	assert.False(t, plan.Empty())
	assert.Equal(t, ActionCreated, plan.Action())
	assert.True(t, plan.Code.Changed())
}

// TestBuild_MissingWorkloadPlansACreate: a workload deleted from under the
// manifest is drift, and the apply for it is a create. The plan says so in one
// line rather than listing every field the file names against an empty live
// object, which is what a comparison would produce and what a reader would
// have to wade through.
func TestBuild_MissingWorkloadPlansACreate(t *testing.T) {
	plan, err := Build(
		loadedFrom(planPayload),
		Live{Live: manifest.Live{WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0"}, State: StateMissing},
		builtCode(0),
		Options{},
	)
	require.NoError(t, err)

	assert.True(t, plan.Creates)
	assert.Equal(t, StateMissing, plan.State)
	assert.Equal(t, ActionCreated, plan.Action())
	assert.Empty(t, plan.Artifact, "a create says so once instead of per field")
	assert.Empty(t, plan.Runtime)
}

// TestBuild_MissingWorkloadKeepsTheDeadID: the id the file was bound to is the
// only thing separating "deleted, recreate it" from "you are pointed at the
// wrong instance", so the plan has to be able to print it.
func TestBuild_MissingWorkloadKeepsTheDeadID(t *testing.T) {
	plan, err := Build(
		loadedFrom(planPayload),
		Live{Live: manifest.Live{WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0"}, State: StateMissing},
		builtCode(0),
		Options{},
	)
	require.NoError(t, err)

	assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", plan.PriorWorkloadID)
}

// TestBuild_TerminatedWorkloadIsNotACreate: a terminated workload still
// exists and still owns its name, so the apply for it is a refusal, not a
// create. Planning one would announce a name the create would not use and a
// binding that has not gone anywhere.
func TestBuild_TerminatedWorkloadIsNotACreate(t *testing.T) {
	plan, err := Build(
		loadedFrom(planPayload),
		Live{Live: manifest.Live{WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0"}, State: StateTerminated},
		builtCode(0),
		Options{},
	)
	require.NoError(t, err)

	assert.False(t, plan.Creates)
	assert.Empty(t, plan.PriorWorkloadID)
}

// TestBuild_FirstDeployHasNoPriorBinding: a create only names an id when it is
// replacing a binding, so an unbound manifest must not print one.
func TestBuild_FirstDeployHasNoPriorBinding(t *testing.T) {
	plan, err := Build(loadedFrom(planPayload), Live{State: StateUnbound}, builtCode(0), Options{})
	require.NoError(t, err)

	assert.True(t, plan.Creates)
	assert.Empty(t, plan.PriorWorkloadID)
}

func TestBuild_CarriesCodeAndStateThrough(t *testing.T) {
	plan, err := Build(
		loadedFrom(planPayload),
		liveFrom(t, StateStopped, planLiveSpec, planLiveRuntime),
		builtCode(7),
		Options{},
	)
	require.NoError(t, err)

	assert.Equal(t, StateStopped, plan.State)
	assert.Equal(t, 7, plan.Code.Files)
	assert.Equal(t, ActionRolled, plan.Action(), "a stopped workload with changed code still rolls")
}

func TestBuild_BadPayloadIsReported(t *testing.T) {
	_, err := Build(loadedFrom("{not json"), Live{State: StateRunning}, builtCode(0), Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compiled manifest")
}

// A file may write the type inside the spec, which the platform accepts and
// the ledger does not forbid. The live artifact still comes back without one,
// so comparing the two blocks reported the type missing on every run: a
// re-deploy of an untouched project minted a version and rolled, then found it
// missing again, with no state anywhere that could end the loop. Reproduced
// against staging before it was fixed.
func TestBuild_TypeInsideTheSpecIsNotPerpetualDrift(t *testing.T) {
	loaded := Loaded{Compiled: &manifest.Compiled{
		Payload: json.RawMessage(`{
		  "name": "my-app",
		  "artifact": {"name": "my-app-artifact", "spec": {"type": "service"}}
		}`),
	}}

	live := liveFrom(t, StateRunning, "", planLiveRuntime)
	live.ArtifactType = "service"

	plan, err := Build(loaded, live, builtCode(0), Options{})
	require.NoError(t, err)

	assert.Empty(t, paths(plan.Artifact), "the file and the workload agree about the type")
	assert.True(t, plan.Empty(), "a re-deploy of an untouched project has nothing to do")
}

// The other half of that fix: reading the type from inside the spec must not
// mean ignoring it. A file that turns a service into an agent there is asking
// for a new lineage exactly as it would beside the spec.
func TestBuild_TypeChangeInsideTheSpecIsStillARoll(t *testing.T) {
	loaded := Loaded{Compiled: &manifest.Compiled{
		Payload: json.RawMessage(`{
		  "name": "my-app",
		  "artifact": {"name": "my-app-artifact", "spec": {"type": "agent"}}
		}`),
	}}

	live := liveFrom(t, StateRunning, "", planLiveRuntime)
	live.ArtifactType = "service"

	plan, err := Build(loaded, live, builtCode(0), Options{})
	require.NoError(t, err)

	assert.Equal(t, []string{"artifact.type"}, paths(plan.Artifact))
	assert.Equal(t, ActionRolled, plan.Action())
}

// Beside the spec wins when a file says both, because that is where the
// platform keeps the answer.
func TestBuild_TypeBesideTheSpecWinsOverTypeWithinIt(t *testing.T) {
	loaded := Loaded{Compiled: &manifest.Compiled{
		Payload: json.RawMessage(`{
		  "name": "my-app",
		  "artifact": {"name": "my-app-artifact", "type": "agent", "spec": {"type": "service"}}
		}`),
	}}

	live := liveFrom(t, StateRunning, "", planLiveRuntime)
	live.ArtifactType = "agent"

	plan, err := Build(loaded, live, builtCode(0), Options{})
	require.NoError(t, err)

	assert.Empty(t, paths(plan.Artifact), "the type beside the spec is the one that counts")
}

// The line the reader sees has to be the comparison that was made. Reporting
// the raw empty string rendered "artifact.type:  -> agent", a change with
// nothing on the left, about a side this code had just decided means service.
func TestBuild_TypeChangeAgainstAnUnstatedLiveTypeReadsAsService(t *testing.T) {
	loaded := Loaded{Compiled: &manifest.Compiled{
		Payload: json.RawMessage(`{
		  "name": "my-app",
		  "artifact": {"name": "my-app-artifact", "type": "agent", "spec": {}}
		}`),
	}}

	live := liveFrom(t, StateRunning, "", planLiveRuntime)
	live.ArtifactType = ""

	plan, err := Build(loaded, live, builtCode(0), Options{})
	require.NoError(t, err)

	require.Len(t, plan.Artifact, 1)
	assert.Equal(t, "service", plan.Artifact[0].Have, "the silence was read as a service, so that is what it says")
	assert.Equal(t, "artifact.type: service -> agent", plan.Artifact[0].String())
}

// An artifact that states no type is a service, because that is what the
// platform defaults it to. Reading the silence as a difference would be drift
// a file saying "service" could never settle.
func TestBuild_LiveArtifactWithNoStatedTypeIsAService(t *testing.T) {
	loaded := Loaded{Compiled: &manifest.Compiled{
		Payload: json.RawMessage(`{
		  "name": "my-app",
		  "artifact": {"name": "my-app-artifact", "type": "service", "spec": {}}
		}`),
	}}

	live := liveFrom(t, StateRunning, "", planLiveRuntime)
	live.ArtifactType = ""

	plan, err := Build(loaded, live, builtCode(0), Options{})
	require.NoError(t, err)

	assert.Empty(t, paths(plan.Artifact))
}

// The type sits beside the spec, not in it: the platform reads the
// discriminator off the artifact and pops any type sent within the spec, so
// the spec walk is blind to it. Without a comparison of its own, turning a
// service into an agent reads as already up to date.
func TestBuild_ArtifactTypeChangeIsARoll(t *testing.T) {
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

	assert.Equal(t, []string{"artifact.type"}, paths(plan.Artifact))
	assert.Equal(t, ActionRolled, plan.Action(), "a different kind of artifact needs a new version")
	assert.False(t, plan.Empty())
}

// The same file against a workload already running that kind plans nothing,
// or every deploy of an agent would roll it onto itself.
func TestBuild_MatchingArtifactTypeIsEmpty(t *testing.T) {
	loaded := Loaded{Compiled: &manifest.Compiled{
		Payload: json.RawMessage(`{
		  "name": "my-app",
		  "artifact": {"name": "my-app-artifact", "type": "agent", "spec": {}}
		}`),
	}}

	live := liveFrom(t, StateRunning, "", planLiveRuntime)
	live.ArtifactType = "agent"

	plan, err := Build(loaded, live, builtCode(0), Options{})
	require.NoError(t, err)

	assert.Empty(t, plan.Artifact)
	assert.True(t, plan.Empty())
}

// A spelling difference is not a change. Nothing in the file can resolve one,
// because the platform normalises the value and hands the same thing back, so
// a case-sensitive comparison here is drift that never clears: every run mints
// a version and rolls a workload nobody touched, for ever. The ledger does not
// enforce an enum on the type, so a hand-written "Service" reaches this.
func TestBuild_ArtifactTypeSpellingIsNotAChange(t *testing.T) {
	loaded := Loaded{Compiled: &manifest.Compiled{
		Payload: json.RawMessage(`{
		  "name": "my-app",
		  "artifact": {"name": "my-app-artifact", "type": "Service", "spec": {}}
		}`),
	}}

	live := liveFrom(t, StateRunning, "", planLiveRuntime)
	live.ArtifactType = "service"

	plan, err := Build(loaded, live, builtCode(0), Options{})
	require.NoError(t, err)

	assert.Empty(t, plan.Artifact)
	assert.True(t, plan.Empty(), "a workload nobody changed is already up to date")
}

// Saying nothing about the type is not the same as asking for a service. A
// manifest written before the type moved out of the spec names none, and this
// walk never reverts what the file leaves out.
func TestBuild_NoTypeInTheFileIsNotDrift(t *testing.T) {
	loaded := Loaded{Compiled: &manifest.Compiled{
		Payload: json.RawMessage(`{
		  "name": "my-app",
		  "artifact": {"name": "my-app-artifact", "spec": {}}
		}`),
	}}

	live := liveFrom(t, StateRunning, "", planLiveRuntime)
	live.ArtifactType = "agent"

	plan, err := Build(loaded, live, builtCode(0), Options{})
	require.NoError(t, err)

	assert.Empty(t, plan.Artifact)
	assert.True(t, plan.Empty())
}

// creates() and deployable() are two switches over the same enum answering two
// halves of one question: which states an apply may proceed from, and which of
// those it creates in rather than reconciles. `exhaustive` catches a new State
// missing from either switch, but not an existing one filed under the wrong
// branch in only one of them, and the two ways that goes wrong are the two
// worst outcomes this change has: creating over a workload that is still there,
// or refusing one that this change exists to recover.
//
// So the relationship is asserted rather than left to two lists staying in step
// by inspection: a state a create can happen in must be a state an apply is
// allowed to proceed from at all.
func TestCreates_IsASubsetOfWhatDeployableAllows(t *testing.T) {
	for _, state := range allStates() {
		if !creates(state) {
			continue
		}

		err := deployable(Live{State: state}, "my-app", "")
		assert.NoError(t, err, "%s creates but is refused, so nothing can act on the plan", state)
	}
}

// The other direction, stated as the classification it is rather than derived,
// so moving a state between the two branches has to be a deliberate edit here
// as well. Terminated is the one that reads like it belongs with Missing and
// does not: the workload still exists, still holds its name and still owns its
// artifact, so a create collides with the name and every line of the plan about
// it would be false.
func TestCreates_ClassifiesEveryState(t *testing.T) {
	want := map[State]bool{
		StateUnbound:    true,
		StateMissing:    true,
		StateTerminated: false,
		StateStopped:    false,
		StateSettling:   false,
		StateRunning:    false,
		StateErrored:    false,
	}

	require.Len(t, want, len(allStates()), "a new State needs a row here")

	for _, state := range allStates() {
		expected, ok := want[state]
		require.True(t, ok, "%s is not classified", state)
		assert.Equal(t, expected, creates(state), "%s", state)
	}
}

// allStates is every value of the enum, so a table over it cannot silently miss
// one that was added since.
func allStates() []State {
	return []State{
		StateUnbound,
		StateMissing,
		StateTerminated,
		StateStopped,
		StateSettling,
		StateRunning,
		StateErrored,
	}
}

// TestBuild_DiffRowsMirrorTheArtifactRuntimeSplit: the diff rows come from
// the same two walks the change lists do, split the same way, so a diff and
// the plan beside it can never disagree about what differs or which block a
// finding belongs to. The agreeing leaves are kept as context, which is what
// makes the rows a diff rather than a second change list.
func TestBuild_DiffRowsMirrorTheArtifactRuntimeSplit(t *testing.T) {
	drifted := `{
	  "name": "my-app",
	  "artifact": {"name": "my-app-artifact", "spec": {"containerGroups": [
	    {"name": "default", "containers": [{"name": "primary", "port": 9090}]}
	  ]}},
	  "runtime": {"containerGroups": [{"name": "default", "replicaCount": 3}]}
	}`

	plan, err := Build(
		loadedFrom(drifted),
		liveFrom(t, StateRunning, planLiveSpec, planLiveRuntime),
		builtCode(0),
 Options{},
	)
	require.NoError(t, err)

	assert.Equal(t, paths(plan.Artifact), rowPaths(changedRows(plan.DiffArtifact)))
	assert.Equal(t, paths(plan.Runtime), rowPaths(changedRows(plan.DiffRuntime)))

	require.Greater(t, len(plan.DiffArtifact), len(plan.Artifact), "the spec rows keep the agreeing leaves")

	var port DiffRow

	for _, r := range plan.DiffArtifact {
		if r.Path == "containerGroups[default].containers[primary].port" && !r.Changed {
			port = r
		}
	}

	assert.False(t, port.Changed, "a leaf the file and the workload agree on is context")
}

// TestBuild_CreatePathComputesAllAdditionRows: a create has no live side to
// compare against, so the diff walks the file against nil and every leaf is
// an addition, walked out per leaf. The default plan's halves stay empty,
// because a create says so once rather than per field.
func TestBuild_CreatePathComputesAllAdditionRows(t *testing.T) {
	plan, err := Build(
		loadedFrom(planPayload),
		Live{State: StateUnbound},
		CodeChange{Applies: true, FirstDeploy: true},
		Options{},
	)
	require.NoError(t, err)

	require.NotEmpty(t, plan.DiffArtifact)

	for _, r := range plan.DiffArtifact {
		assert.True(t, r.Absent, "row %s", r.Path)
		assert.True(t, r.Changed, "row %s", r.Path)
		assert.Nil(t, r.Have, "row %s", r.Path)
	}

	require.NotEmpty(t, plan.DiffRuntime)

	for _, r := range plan.DiffRuntime {
		assert.True(t, r.Absent, "row %s", r.Path)
		assert.True(t, r.Changed, "row %s", r.Path)
	}

	assert.Empty(t, plan.Unmanaged, "nothing is live, so nothing is unmanaged")
	assert.Empty(t, plan.Artifact)
	assert.Empty(t, plan.Runtime)
}

// TestBuild_UnmanagedComesFromExtra: what the live object carries that the
// file never names lands on the plan as paths, so the diff can count it.
// The same element is unmanaged in both documents here -- the metrics
// sidecar exists in the artifact's spec and in the workload's runtime -- and
// one element is one field to a reader, so the list holds the path once.
func TestBuild_UnmanagedComesFromExtra(t *testing.T) {
	plan, err := Build(
		loadedFrom(planPayload),
		liveFrom(t, StateRunning, planLiveSpec, planLiveRuntime),
		builtCode(0),
 Options{},
	)
	require.NoError(t, err)

	assert.Equal(t,
		[]string{"containerGroups[default].containers[metrics]"},
		plan.Unmanaged)
}

// TestBuild_SyntheticArtifactIDChangeEntersTheDiffRows: the artifact id is
// not a leaf of the spec, so no walk of it can produce the change. It is
// merged into the rows all the same, because a change the default plan
// prints must not be invisible in the diff.
func TestBuild_SyntheticArtifactIDChangeEntersTheDiffRows(t *testing.T) {
	loaded := Loaded{Compiled: &manifest.Compiled{
		Payload:    json.RawMessage(`{"name": "my-app", "artifactId": "68b0bbbb0000000000000002"}`),
		ArtifactID: "68b0bbbb0000000000000002",
	}}

	live := liveFrom(t, StateRunning, "", planLiveRuntime)
	live.ArtifactID = "68a0000000000000000000a1"

	plan, err := Build(loaded, live, builtCode(0), Options{})
	require.NoError(t, err)

	changed := changedRows(plan.DiffArtifact)

	require.Len(t, changed, 1)
	assert.Equal(t, "artifactId", changed[0].Path)
	assert.Equal(t, "68a0000000000000000000a1", changed[0].Have)
	assert.Equal(t, "68b0bbbb0000000000000002", changed[0].Want)

	out := renderDiff(t, appSummary, plan)

	assert.Contains(t, out, "- artifactId: 68a0000000000000000000a1")
	assert.Contains(t, out, "+ artifactId: 68b0bbbb0000000000000002")
}

// The type sits beside the spec for the same reason, and lands in the rows
// the same way, with the defaulted live value on the left of the pair.
func TestBuild_SyntheticArtifactTypeChangeEntersTheDiffRows(t *testing.T) {
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

	changed := changedRows(plan.DiffArtifact)

	require.Len(t, changed, 1)
	assert.Equal(t, "artifact.type", changed[0].Path)
	assert.Equal(t, "service", changed[0].Have)
	assert.Equal(t, "agent", changed[0].Want)

	out := renderDiff(t, appSummary, plan)

	assert.Contains(t, out, "- artifact.type: service")
	assert.Contains(t, out, "+ artifact.type: agent")
}
