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
	"testing"

	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadedFrom builds a Loaded around a compiled payload, which is the only
// part of it Build reads.
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
		{"nothing differs", Plan{Code: builtCode(0)}, false},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, c.plan.RollsArtifact(), c.name)
	}
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

	plan, err := Build(loaded, live, builtCode(0))
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

	plan, err := Build(loaded, live, builtCode(0))
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
	)
	require.NoError(t, err)

	assert.True(t, plan.Creates)
	assert.Empty(t, plan.Artifact)
	assert.Empty(t, plan.Runtime)
	assert.False(t, plan.Empty())
	assert.Equal(t, ActionCreated, plan.Action())
	assert.True(t, plan.Code.Changed())
}

// TestBuild_MissingWorkloadStillPlans: a workload deleted from under the
// manifest is refused at apply time, but --dry-run still has to say what
// would have happened, so the plan is computed rather than short-circuited.
func TestBuild_MissingWorkloadStillPlans(t *testing.T) {
	plan, err := Build(
		loadedFrom(planPayload),
		Live{State: StateMissing},
		builtCode(0),
	)
	require.NoError(t, err)

	assert.False(t, plan.Creates, "a vanished workload is not a licence to create a second one")
	assert.Equal(t, StateMissing, plan.State)
	assert.NotEmpty(t, plan.Artifact, "with no live spec, everything the file names is an addition")
}

func TestBuild_CarriesCodeAndStateThrough(t *testing.T) {
	plan, err := Build(
		loadedFrom(planPayload),
		liveFrom(t, StateStopped, planLiveSpec, planLiveRuntime),
		builtCode(7),
	)
	require.NoError(t, err)

	assert.Equal(t, StateStopped, plan.State)
	assert.Equal(t, 7, plan.Code.Files)
	assert.Equal(t, ActionRolled, plan.Action(), "a stopped workload with changed code still rolls")
}

func TestBuild_BadPayloadIsReported(t *testing.T) {
	_, err := Build(loadedFrom("{not json"), Live{State: StateRunning}, builtCode(0))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compiled manifest")
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

	plan, err := Build(loaded, live, builtCode(0))
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

	plan, err := Build(loaded, live, builtCode(0))
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

	plan, err := Build(loaded, live, builtCode(0))
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

	plan, err := Build(loaded, live, builtCode(0))
	require.NoError(t, err)

	assert.Empty(t, plan.Artifact)
	assert.True(t, plan.Empty())
}
