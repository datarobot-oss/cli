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

package manifest

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// liveWorkloadJSON and liveArtifactJSON are shaped like the server's own
// responses, including the things the wizard has no vocabulary for:
// autoscaling, a GPU bundle, a second container, a credential-backed
// variable. Those are exactly what binding must not lose.
const liveWorkloadJSON = `{
  "id": "68b0c1d2e3f4a5b6c7d8e9f0",
  "name": "gpt-oss-20b-vllm",
  "status": "running",
  "importance": "moderate",
  "artifactId": "68a0000000000000000000a1",
  "endpoint": "https://app.datarobot.com/workloads/68b0/",
  "createdAt": "2026-07-02T10:00:00Z",
  "runtime": {
    "containerGroups": [
      {
        "name": "default",
        "resourceBundles": ["gpu.medium"],
        "autoscaling": {
          "enabled": true,
          "minReplicaCount": 1,
          "maxReplicaCount": 4,
          "policies": [{"scalingMetric": "gpuCacheUtilization", "target": 70}]
        },
        "containers": [
          {"name": "vllm-server", "resourceAllocation": {"cpu": 3, "memory": "22GB", "gpu": 1}},
          {"name": "metrics-bridge", "resourceAllocation": {"cpu": 0.5, "memory": "2GB"}}
        ]
      }
    ]
  }
}`

const liveArtifactJSON = `{
  "id": "68a0000000000000000000a1",
  "name": "gpt-oss-20b-vllm-artifact",
  "status": "locked",
  "createdAt": "2026-07-02T09:00:00Z",
  "spec": {
    "type": "service",
    "containerGroups": [
      {
        "name": "default",
        "containers": [
          {
            "name": "vllm-server",
            "primary": true,
            "port": 8000,
            "imageUri": "registry.internal/built-by-the-server:sha-abc123",
            "imageBuildConfig": {
              "dockerfile": {"source": "provided"},
              "codeRef": {"datarobot": {"catalogId": "cat1", "catalogVersionId": "ver1"}}
            },
            "environmentVars": [
              {"name": "HF_HOME", "value": "/tmp/hf"},
              {"name": "HUGGING_FACE_HUB_TOKEN", "source": "dr-credential",
               "drCredentialId": "66f1a2b3c4d5e6f7a8b9c0d1", "key": "apiToken"}
            ],
            "readinessProbe": {"path": "/health", "port": 8000, "periodSeconds": 10},
            "startupProbe": {"path": "/health", "port": 8000, "periodSeconds": 15, "failureThreshold": 60}
          },
          {"name": "metrics-bridge", "imageUri": "registry/vllm-metrics-bridge:v1"}
        ]
      }
    ]
  }
}`

func liveFixture(t *testing.T) Live {
	t.Helper()

	var workloadDoc, artifactDoc map[string]any

	require.NoError(t, json.Unmarshal([]byte(liveWorkloadJSON), &workloadDoc))
	require.NoError(t, json.Unmarshal([]byte(liveArtifactJSON), &artifactDoc))

	live, err := NewLive("68b0c1d2e3f4a5b6c7d8e9f0", workloadDoc, artifactDoc)
	require.NoError(t, err)

	return live
}

func TestNewLive_StripsWhatOnlyTheServerSets(t *testing.T) {
	live := liveFixture(t)
	rendered, err := live.Render()
	require.NoError(t, err)

	for _, unwanted := range []string{"createdAt", "68a0000000000000000000a1", "app.datarobot.com", "status: locked"} {
		assert.NotContains(t, string(rendered), unwanted)
	}

	// The binding itself is the one id the file carries.
	assert.Contains(t, string(rendered), "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0")
}

// The image a build produced and the code the last sync uploaded are earned
// again by the next deploy. Writing them down would pin the workload to
// yesterday's image.
func TestNewLive_StripsBuildOutputs(t *testing.T) {
	rendered, err := liveFixture(t).Render()
	require.NoError(t, err)

	assert.NotContains(t, string(rendered), "built-by-the-server")
	assert.NotContains(t, string(rendered), "codeRef")
	assert.Contains(t, string(rendered), "source: provided")

	// A container that genuinely runs a published image keeps it.
	assert.Contains(t, string(rendered), "registry/vllm-metrics-bridge:v1")
}

// Everything the wizard has no question for still has to survive, because
// the file it writes is what the next deploy applies.
func TestLive_RenderKeepsWhatTheWizardCannotSay(t *testing.T) {
	rendered, err := liveFixture(t).Render()
	require.NoError(t, err)

	for _, kept := range []string{
		"autoscaling", "gpuCacheUtilization", "resourceBundles", "gpu.medium",
		"metrics-bridge", "startupProbe", "dr-credential", "HUGGING_FACE_HUB_TOKEN",
	} {
		assert.Contains(t, string(rendered), kept, "binding must not drop %s", kept)
	}
}

func TestLive_DefaultsComeFromTheLiveSpec(t *testing.T) {
	defaults := liveFixture(t).Defaults()

	assert.Equal(t, "gpt-oss-20b-vllm", defaults.Name)
	assert.Equal(t, "moderate", defaults.Importance)
	assert.Equal(t, TypeService, defaults.Type)
	assert.Equal(t, 8000, defaults.Port)
	assert.Equal(t, "/health", defaults.HealthPath)
	assert.Equal(t, BuildModeDockerfile, defaults.Build.Mode)
	assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", defaults.WorkloadID)
}

func TestLive_ApplyChangesOnlyWhatWasAsked(t *testing.T) {
	live := liveFixture(t)

	draft := live.Defaults()
	draft.Port = 9000
	draft.HealthPath = "/ready"
	draft.Build = Build{Mode: BuildModeImage, ImageURI: "registry/team/app:v2"}

	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	assert.Contains(t, string(rendered), "port: 9000")
	assert.Contains(t, string(rendered), "path: /ready")
	assert.Contains(t, string(rendered), "imageUri: registry/team/app:v2")
	// The build config it replaced is gone, so the container names one source.
	assert.NotContains(t, string(rendered), "imageBuildConfig")
	// And the parts nobody asked about are still there.
	assert.Contains(t, string(rendered), "autoscaling")
	assert.Contains(t, string(rendered), "metrics-bridge")

	// Apply leaves the original alone so the confirm screen can diff them.
	before, err := live.Render()
	require.NoError(t, err)
	assert.Contains(t, string(before), "port: 8000")
}

func TestLive_ApplySwitchesToAGeneratedBuild(t *testing.T) {
	live := liveFixture(t)

	draft := live.Defaults()
	draft.Build = Build{
		Mode:                          BuildModeGenerated,
		ExecutionEnvironmentID:        "68a1",
		ExecutionEnvironmentVersionID: "68a2",
		Entrypoint:                    []string{"python", "main.py"},
	}

	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	assert.Contains(t, string(rendered), "source: generated")
	assert.Contains(t, string(rendered), "executionEnvironmentId: 68a1")
	assert.Contains(t, string(rendered), "- python")
}

func TestLive_ApplyTogglesA2A(t *testing.T) {
	live := liveFixture(t)

	draft := live.Defaults()
	draft.Type = TypeAgent
	draft.A2AEnabled = true

	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	assert.Contains(t, string(rendered), "type: agent")
	assert.Contains(t, string(rendered), "a2aEnabled: true")

	draft.A2AEnabled = false

	applied, err = live.Apply(draft)
	require.NoError(t, err)

	rendered, err = applied.Render()
	require.NoError(t, err)
	assert.NotContains(t, string(rendered), "a2aEnabled")
}

// A downloaded workload has to produce a file this CLI can read back and
// deploy, or binding has just written a dead end.
func TestLive_RenderRoundTripsThroughValidate(t *testing.T) {
	live := liveFixture(t)

	draft := live.Defaults()

	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	// The live spec builds a provided Dockerfile, so the ledger's file check
	// needs one; "" skips the file-existence rules the same way a preview does.
	parsed, err := Parse(rendered, "")
	require.NoError(t, err)
	require.NoError(t, parsed.Validate())

	compiled, err := parsed.Compile()
	require.NoError(t, err)

	assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", compiled.WorkloadID)
	require.Len(t, compiled.CredentialRefs, 1)
	assert.Equal(t, "66f1a2b3c4d5e6f7a8b9c0d1", compiled.CredentialRefs[0].CredentialID)
}

// A spec with no container to update is a workload the wizard cannot honestly
// edit, so it says so instead of writing something plausible.
func TestLive_ApplyWithoutAPrimaryContainer(t *testing.T) {
	// The spec carries containerGroups but no containers, so NewLive accepts
	// it (the spec is non-empty) while Apply finds no primary container to
	// update.
	live, err := NewLive("68b0", map[string]any{"name": "empty"}, map[string]any{
		"name": "empty-artifact",
		"spec": map[string]any{"containerGroups": []any{}},
	})
	require.NoError(t, err)

	_, err = live.Apply(live.Defaults())
	require.Error(t, err)

	assert.Contains(t, err.Error(), "no primary container")
}

// LiteralEnvVars answers one question: which of the workload's variables would
// a bind copy with the value in the open. A credential-backed entry would not,
// in either spelling, and reporting one as a literal would send the caller
// looking for a leak that is not there.
func TestLive_LiteralEnvVarsSkipsCredentialReferences(t *testing.T) {
	var artifactDoc map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
      "name": "a",
      "spec": {"containerGroups": [{"name": "default", "containers": [
        {"name": "primary", "primary": true, "port": 8080, "imageUri": "nginx:latest",
         "environmentVars": [
           {"name": "LOG_LEVEL", "value": "debug"},
           {"name": "SHORTHAND", "value": "dr-credential:68f0cccc0000000000000003/apiToken"},
           {"name": "OBJECT", "source": "dr-credential",
            "drCredentialId": "68f0cccc0000000000000003", "key": "apiToken"},
           {"name": "REGION", "value": "eu-west-1"}
         ]}
      ]}]}
    }`), &artifactDoc))

	live, err := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)
	require.NoError(t, err)

	assert.Equal(t, []EnvVar{
		{Name: "LOG_LEVEL", Value: "debug"},
		{Name: "REGION", Value: "eu-west-1"},
	}, live.LiteralEnvVars())
}

// A spec with no container has nothing declared, which is not the same as
// having something this cannot read.
func TestLive_LiteralEnvVarsWithoutAContainer(t *testing.T) {
	// The spec carries containerGroups but no containers, so NewLive accepts
	// it while LiteralEnvVars finds no primary container to read from.
	live, err := NewLive("68b0", map[string]any{"name": "empty"}, map[string]any{
		"name": "empty-artifact",
		"spec": map[string]any{"containerGroups": []any{}},
	})
	require.NoError(t, err)

	assert.Empty(t, live.LiteralEnvVars())
}

// An unflagged spec still has a container traffic reaches: the first one.
func TestLive_PrimaryFallsBackToTheFirstContainer(t *testing.T) {
	var artifactDoc map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
      "name": "a",
      "spec": {"containerGroups": [{"name": "default", "containers": [
        {"name": "only", "port": 8080, "imageUri": "nginx:latest"}
      ]}]}
    }`), &artifactDoc))

	live, err := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)
	require.NoError(t, err)

	assert.Equal(t, 8080, live.Defaults().Port)
}

// A build block can carry more than the wizard models (build arguments, a
// timeout, whatever the platform adds next). Rebuilding it from the Build
// struct would drop all of it on a run that changed nothing.
func TestLive_ApplyKeepsUnknownBuildKeys(t *testing.T) {
	var artifactDoc map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
      "name": "a",
      "type": "service", "spec": {"containerGroups": [{"name": "default", "containers": [
        {"name": "primary", "primary": true, "port": 8080, "imageBuildConfig": {
          "dockerfile": {"source": "provided", "buildArgs": {"NODE_ENV": "production"}, "target": "runtime"},
          "buildTimeoutSeconds": 3600,
          "someFuturePlatformKey": {"nested": true}
        }}]}]}
    }`), &artifactDoc))

	live, err := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)
	require.NoError(t, err)

	applied, err := live.Apply(live.Defaults())
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	for _, kept := range []string{"buildArgs", "NODE_ENV", "target", "buildTimeoutSeconds", "someFuturePlatformKey"} {
		assert.Contains(t, string(rendered), kept, "changing nothing must not drop %s", kept)
	}
}

// A build source this release does not know is kept as it stands rather than
// forced into the nearest mode the Build struct can express.
//
// Validate is intentionally NOT exercised on this hypothetical shape. Today
// the server only accepts provided/generated dockerfile sources
// (containers.py:84-95 discriminated union), so a buildpack-only
// imageBuildConfig cannot arrive from a real payload. Under the pass-through
// contract the unknown source is preserved for the server to judge, not
// blessed by the CLI — and Validate's dockerfile-required check would reject
// it (the container carries an imageBuildConfig with no dockerfile key). The
// test asserts the pass-through behavior (preservation) only, not that the
// rendered file validates.
func TestLive_UnknownBuildSourceIsPreserved(t *testing.T) {
	var artifactDoc map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
      "name": "a",
      "type": "service", "spec": {"containerGroups": [{"name": "default", "containers": [
        {"name": "primary", "primary": true, "port": 8080,
         "imageBuildConfig": {"buildpack": {"builder": "paketo/builder:base"}}}]}]}
    }`), &artifactDoc))

	live, err := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)
	require.NoError(t, err)

	assert.Equal(t, BuildModeUnchanged, live.Defaults().Build.Mode)

	applied, err := live.Apply(live.Defaults())
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	assert.Contains(t, string(rendered), "buildpack")
	assert.Contains(t, string(rendered), "paketo/builder:base")
	assert.NotContains(t, string(rendered), `imageUri: ""`, "an empty image must never be invented")
}

// A workload deliberately running without a readiness probe must not acquire
// the wizard's default one, which its container would never pass. The rule
// lives in Defaults, which reads the absence as an empty path, so nothing
// downstream has to tell a default apart from an answer.
func TestLive_ApplyDoesNotInventAReadinessProbe(t *testing.T) {
	live := probelessLive(t)

	require.Empty(t, live.Defaults().HealthPath, "no probe has to arrive as no path")

	applied, err := live.Apply(live.Defaults())
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	assert.NotContains(t, string(rendered), "readinessProbe")
}

// Asking for one is the other half of that: a path only reaches Apply because
// a screen or a flag put it there, and the confirm screen shows the diff before
// anything is written.
func TestLive_ApplyAddsTheProbeThatWasAskedFor(t *testing.T) {
	live := probelessLive(t)

	draft := live.Defaults()
	draft.HealthPath = "/ready"

	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	// Asserted as the block rather than as two loose lines: the container
	// carries a port of its own, so "port: 9000" alone passes whether or not
	// the probe ever got one.
	assert.Contains(t, string(rendered), "readinessProbe:\n              path: /ready\n              port: 9000")
}

// And clearing the path takes the block away rather than leaving one pointed
// at nothing, which is neither a probe nor an absence of one.
func TestLive_ApplyRemovesADeclinedProbe(t *testing.T) {
	var artifactDoc map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
      "name": "a",
      "type": "service", "spec": {"containerGroups": [{"name": "default", "containers": [
        {"name": "primary", "primary": true, "port": 9000, "imageUri": "registry/a:v1",
         "readinessProbe": {"path": "/health", "port": 9000, "periodSeconds": 10}}]}]}
    }`), &artifactDoc))

	live, err := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)
	require.NoError(t, err)

	draft := live.Defaults()
	require.Equal(t, "/health", draft.HealthPath)

	draft.HealthPath = ""

	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	assert.NotContains(t, string(rendered), "readinessProbe")
	assert.NotContains(t, string(rendered), "periodSeconds", "the whole block goes, not only the path")
}

// A probe this release cannot read a path out of is not the same thing as no
// probe, and Apply must not confuse the two: reading it as "declined" would
// delete a running workload's own configuration because the reader did not
// recognize its shape.
func TestLive_ApplyKeepsAProbeItCannotRead(t *testing.T) {
	var artifactDoc map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
      "name": "a",
      "type": "service", "spec": {"containerGroups": [{"name": "default", "containers": [
        {"name": "primary", "primary": true, "port": 9000, "imageUri": "registry/a:v1",
         "readinessProbe": {"tcpSocket": {"port": 9000}, "periodSeconds": 10, "failureThreshold": 30}}]}]}
    }`), &artifactDoc))

	live, err := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)
	require.NoError(t, err)

	present, readable := live.ReadinessProbe()
	assert.True(t, present, "the block is there")
	assert.False(t, readable, "but not as a path this release understands")

	// The wizard's own answers, unchanged: nobody declined anything.
	require.Empty(t, live.Defaults().HealthPath)

	applied, err := live.Apply(live.Defaults())
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	assert.Contains(t, string(rendered), "tcpSocket")
	assert.Contains(t, string(rendered), "failureThreshold: 30")
	assert.NotContains(t, string(rendered), "path:", "nothing was added to a probe we do not model")
}

// And when the container's port moves, that probe follows it the way the
// startup and liveness probes do, rather than being left watching a port
// nothing listens on.
func TestLive_ApplyMovesAnUnreadableProbeWithThePort(t *testing.T) {
	var artifactDoc map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
      "name": "a",
      "type": "service", "spec": {"containerGroups": [{"name": "default", "containers": [
        {"name": "primary", "primary": true, "port": 9000, "imageUri": "registry/a:v1",
         "readinessProbe": {"tcpSocket": {"port": 9000}, "port": 9000}}]}]}
    }`), &artifactDoc))

	live, err := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)
	require.NoError(t, err)

	draft := live.Defaults()
	draft.Port = 9100

	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	// Both ports follow: the probe's own, and the one the tcpSocket shape keeps
	// a level down. A probe still dialling 9000 would never report ready.
	assert.Contains(t, string(rendered), "readinessProbe:\n              port: 9100")
	assert.Contains(t, string(rendered), "tcpSocket:\n                port: 9100")
	assert.NotContains(t, string(rendered), "port: 9000")
}

// probelessLive is a workload running without a readiness probe.
func probelessLive(t *testing.T) Live {
	t.Helper()

	var artifactDoc map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
      "name": "a",
      "type": "service", "spec": {"containerGroups": [{"name": "default", "containers": [
        {"name": "primary", "primary": true, "port": 9000, "imageUri": "registry/a:v1"}]}]}
    }`), &artifactDoc))

	live, err := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)
	require.NoError(t, err)

	return live
}

// A workload can carry probes the wizard has no question for. Changing the
// port has to take them along: a startup probe left behind on the old port is
// a workload that builds, starts, and then never reports ready.
func TestLive_ApplyMovesPreservedProbesWithThePort(t *testing.T) {
	live := liveFixture(t)

	draft := live.Defaults()
	draft.Port = 9000

	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	assert.Contains(t, string(rendered), keyStartupProbe)
	assert.NotContains(t, string(rendered), "8000", "every probe watching the container's port follows it")
	assert.Contains(t, string(rendered), "failureThreshold: 60", "the rest of the probe is untouched")
}

// A probe on a port of its own was pointed there deliberately, so it stays
// where it is rather than being dragged along with the container's port.
func TestLive_ApplyLeavesAProbeOnItsOwnPort(t *testing.T) {
	var artifactDoc map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
      "name": "a",
      "type": "service", "spec": {"containerGroups": [{"name": "default", "containers": [
        {"name": "primary", "primary": true, "port": 8080, "imageUri": "registry/a:v1",
         "livenessProbe": {"path": "/admin/live", "port": 9900}}]}]}
    }`), &artifactDoc))

	live, err := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)
	require.NoError(t, err)

	draft := live.Defaults()
	draft.Port = 9000

	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	assert.Contains(t, string(rendered), "port: 9900")
}

// A spec that flags no primary container still has one traffic reaches, and
// the file must say so or the ledger rejects the port it just wrote.
func TestLive_ApplyFlagsTheContainerItEdits(t *testing.T) {
	var artifactDoc map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
      "name": "a",
      "type": "service", "spec": {"containerGroups": [{"name": "default", "containers": [
        {"name": "only", "port": 8080, "imageUri": "registry/a:v1"}]}]}
    }`), &artifactDoc))

	live, err := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)
	require.NoError(t, err)

	applied, err := live.Apply(live.Defaults())
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	parsed, err := Parse(rendered, "")
	require.NoError(t, err)
	require.NoError(t, parsed.Validate(), "the written file must satisfy the reader that wrote it")
}

// The confirm screen reads its runtime line from here, so it has to describe
// the workload rather than the documented defaults.
func TestLive_DefaultsReadTheLiveSizing(t *testing.T) {
	runtime := liveFixture(t).Defaults().Runtime

	assert.Equal(t, 0, runtime.Replicas, "an autoscaled group has no replica count to report")
	assert.InEpsilon(t, 3.0, runtime.CPU, 0.0001)
	assert.Equal(t, "22GB", runtime.Memory)
}

// The runtime list is joined to the artifact by name, not by position. Reading
// the sizing from whichever container the server happened to list first would
// show the user a sidecar's allocation, and accepting it would then write that
// onto the primary and shrink what is running.
func TestLive_DefaultsReadThePrimaryNotTheFirstListed(t *testing.T) {
	var workloadDoc, artifactDoc map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
      "name": "sidecar-first",
      "runtime": {"containerGroups": [{"name": "default", "replicaCount": 2, "containers": [
        {"name": "metrics-bridge", "resourceAllocation": {"cpu": 0.25, "memory": "256MB"}},
        {"name": "app", "resourceAllocation": {"cpu": 4, "memory": "8GB"}}]}]}
    }`), &workloadDoc))

	require.NoError(t, json.Unmarshal([]byte(`{
      "name": "sidecar-first-artifact",
      "type": "service", "spec": {"containerGroups": [{"name": "default", "containers": [
        {"name": "app", "primary": true, "port": 8080, "imageUri": "example/app:v1"},
        {"name": "metrics-bridge", "imageUri": "example/bridge:v1"}]}]}
    }`), &artifactDoc))

	live, err := NewLive("68b0", workloadDoc, artifactDoc)
	require.NoError(t, err)

	draft := live.Defaults()
	assert.InEpsilon(t, 4.0, draft.Runtime.CPU, 0.0001)
	assert.Equal(t, "8GB", draft.Runtime.Memory)

	// Accepting what was shown leaves both containers exactly as they were.
	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	assert.Contains(t, string(rendered), "cpu: 4")
	assert.Contains(t, string(rendered), "memory: 8GB")
	assert.Contains(t, string(rendered), "cpu: 0.25", "the sidecar keeps its own sizing")
	assert.Contains(t, string(rendered), "memory: 256MB")
}

// An autoscaling block that is switched off is not autoscaling, which is the
// rule both the create-payload check and the manifest ledger already apply.
// Live dissenting from it dropped the user's replica answer for a workload
// that was not autoscaling at all, and said nothing about it.
func TestLive_DisabledAutoscalingKeepsTheReplicaAnswer(t *testing.T) {
	var workloadDoc, artifactDoc map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
      "name": "paused-autoscaling",
      "runtime": {"containerGroups": [{"name": "default", "replicaCount": 3,
        "autoscaling": {"enabled": false, "minReplicaCount": 1, "maxReplicaCount": 4},
        "containers": [{"name": "app", "resourceAllocation": {"cpu": 1, "memory": "1GB"}}]}]}
    }`), &workloadDoc))

	require.NoError(t, json.Unmarshal([]byte(`{
      "name": "a",
      "type": "service", "spec": {"containerGroups": [{"name": "default", "containers": [
        {"name": "app", "primary": true, "port": 8080, "imageUri": "example/app:v1"}]}]}
    }`), &artifactDoc))

	live, err := NewLive("68b0", workloadDoc, artifactDoc)
	require.NoError(t, err)

	draft := live.Defaults()
	assert.Equal(t, 3, draft.Runtime.Replicas, "the group's own count, not the autoscaled zero")

	draft.Runtime.Replicas = 9

	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	assert.Contains(t, string(rendered), "replicaCount: 9")
	assert.Contains(t, string(rendered), "enabled: false", "the policy is preserved as it stands")

	parsed, err := Parse(rendered, "")
	require.NoError(t, err)
	require.NoError(t, parsed.Validate(), "the ledger allows a count alongside disabled autoscaling")
}

// Environment variables merge rather than replace. A name the workload already
// declares keeps its running value, because .env is the developer's copy and
// may well be stale, while a name only .env knows about is appended.
func TestLive_ApplyMergesEnvVars(t *testing.T) {
	live := liveFixture(t)

	draft := live.Defaults()
	draft.EnvVars = []EnvVar{
		{Name: "HF_HOME", Value: "/home/dev/.cache/hf"},
		{Name: "LOG_LEVEL", Value: "debug"},
		{Name: "OPENAI_API_KEY", Secret: true},
	}

	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	assert.Contains(t, string(rendered), "value: /tmp/hf", "the running value wins over the local copy")
	assert.NotContains(t, string(rendered), "/home/dev/.cache/hf")
	assert.Contains(t, string(rendered), "LOG_LEVEL")
	assert.Contains(t, string(rendered), "value: debug")
	assert.Contains(t, string(rendered), "dr-credential:"+CredentialPlaceholder+"/apiToken")

	// Both reference forms have to survive to the payload: the object form the
	// server sent, and the shorthand this merge just wrote.
	parsed, err := Parse(rendered, "")
	require.NoError(t, err)

	compiled, err := parsed.Compile()
	require.NoError(t, err)

	referenced := make([]string, 0, len(compiled.CredentialRefs))
	for _, ref := range compiled.CredentialRefs {
		referenced = append(referenced, ref.EnvName)
	}

	assert.ElementsMatch(t, []string{"HUGGING_FACE_HUB_TOKEN", "OPENAI_API_KEY"}, referenced)
}

// BuildMode reads a parsed manifest rather than a live document: setup asks it
// what the file already says before offering to change it. Driven through
// Render so the reader and the writer are held to the same vocabulary.
func TestManifest_BuildMode(t *testing.T) {
	generated := Build{
		Mode:                          BuildModeGenerated,
		ExecutionEnvironmentID:        "68a1b2c3d4e5f6a7b8c9d0e1",
		ExecutionEnvironmentVersionID: "68a1b2c3d4e5f6a7b8c9d0e2",
		Entrypoint:                    []string{"python", "main.py"},
	}

	for want, build := range map[string]Build{
		BuildModeDockerfile: {Mode: BuildModeDockerfile},
		BuildModeGenerated:  generated,
		BuildModeImage:      {Mode: BuildModeImage, ImageURI: "nginx:latest"},
	} {
		t.Run(want, func(t *testing.T) {
			draft := serviceDraft()
			draft.Build = build

			out, err := draft.Render()
			require.NoError(t, err)

			m, err := Parse(out, "")
			require.NoError(t, err)

			assert.Equal(t, want, m.BuildMode())
		})
	}
}

// A manifest bound to an artifact by id carries no inline spec to read.
func TestManifest_BuildModeWithoutAnInlineArtifact(t *testing.T) {
	m, err := Parse([]byte("name: my-app\nartifactId: 68a0000000000000000000a1\n"), "")
	require.NoError(t, err)

	assert.Empty(t, m.BuildMode())
}

// Sizing answers reach the runtime block, and an autoscaled group keeps its
// autoscaling rather than gaining a replica count the ledger would reject.
func TestLive_ApplyRuntime(t *testing.T) {
	live := liveFixture(t)

	draft := live.Defaults()
	draft.Runtime = Runtime{Replicas: 9, CPU: 16, Memory: "32GB"}

	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	assert.Contains(t, string(rendered), "cpu: 16")
	assert.Contains(t, string(rendered), "memory: 32GB")
	assert.NotContains(t, string(rendered), "replicaCount", "an autoscaled group must not gain one")
	assert.Contains(t, string(rendered), "maxReplicaCount: 4", "the autoscaling policy is untouched")
	assert.Contains(t, string(rendered), "metrics-bridge")
	assert.Contains(t, string(rendered), "2GB", "the second container's allocation is untouched")

	parsed, err := Parse(rendered, "")
	require.NoError(t, err)
	require.NoError(t, parsed.Validate())
}

// stripServerManaged deep-copies the document and strips both server-managed
// keys and nil-valued keys in a single recursive walk, without discarding
// falsy but present values and without mutating the input document.
func TestStripServerManaged_StripsNullKeys(t *testing.T) {
	tests := map[string]struct {
		input    map[string]any
		expected map[string]any
	}{
		"nil map": {
			input:    nil,
			expected: nil,
		},
		"empty map": {
			input:    map[string]any{},
			expected: map[string]any{},
		},
		"nil value removed": {
			input:    map[string]any{"keep": "x", "drop": nil},
			expected: map[string]any{"keep": "x"},
		},
		"falsy values kept": {
			input: map[string]any{
				"false":      false,
				"zero":       0,
				"empty":      "",
				"emptySlice": []any{},
			},
			expected: map[string]any{
				"false":      false,
				"zero":       0,
				"empty":      "",
				"emptySlice": []any{},
			},
		},
		"nested map nil removed": {
			input: map[string]any{
				"nested": map[string]any{
					"keep": "x",
					"drop": nil,
				},
			},
			expected: map[string]any{
				"nested": map[string]any{
					"keep": "x",
				},
			},
		},
		"slice map nil removed": {
			input: map[string]any{
				"items": []any{
					map[string]any{"keep": "x", "drop": nil},
					map[string]any{"keep": "y"},
				},
			},
			expected: map[string]any{
				"items": []any{
					map[string]any{"keep": "x"},
					map[string]any{"keep": "y"},
				},
			},
		},
		// Server-managed keys are also stripped alongside nils, confirming the
		// two passes share the single recursive walk.
		"server-managed keys stripped": {
			input: map[string]any{
				"id":     "68b0",
				"keep":   "x",
				"status": "running",
				"drop":   nil,
			},
			expected: map[string]any{
				"keep": "x",
			},
		},
		// A non-null enabled value is left untouched: false stays false,
		// true stays true. The nil-purge only drops keys whose value is nil.
		"autoscaling enabled false untouched": {
			input: map[string]any{
				"autoscaling": map[string]any{"enabled": false},
			},
			expected: map[string]any{
				"autoscaling": map[string]any{"enabled": false},
			},
		},
		// A genuinely empty autoscaling: {} map (enabled key absent entirely)
		// must survive stripKeys unchanged — the nil-purge only drops
		// nil-valued keys, and an empty map has none.
		"autoscaling empty map untouched": {
			input: map[string]any{
				"autoscaling": map[string]any{},
			},
			expected: map[string]any{
				"autoscaling": map[string]any{},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			original := deepCopyMap(tc.input)
			got := stripServerManaged(tc.input)

			assert.Equal(t, tc.expected, got)
			assert.Equal(t, original, tc.input, "stripServerManaged must not mutate input")
		})
	}
}

// reporterWorkloadDoc returns a workload GET response shaped like the reporter's:
// a runtime container group with null autoscaling and a resolved bundle.
func reporterWorkloadDoc(t *testing.T) map[string]any {
	t.Helper()

	const doc = `{
		"id": "68b0c1d2e3f4a5b6c7d8e9f0",
		"name": "reporter-workload",
		"importance": "low",
		"artifactId": "68a0000000000000000000a1",
		"runtime": {
			"containerGroups": [
				{
					"name": "default",
					"replicaCount": 3,
					"autoscaling": null,
					"resolvedBundle": {"catalogId": "cat1", "catalogVersionId": "ver1"},
					"containers": [
						{"name": "app", "resourceAllocation": {"cpu": 1, "memory": "1GB"}}
					]
				}
			]
		}
	}`

	var out map[string]any

	require.NoError(t, json.Unmarshal([]byte(doc), &out))

	return out
}

// reporterArtifactDoc returns an artifact GET response shaped like the reporter's:
// a container carrying a server build block, imageOutdated flag, imageUri and
// an imageBuildConfig with a codeRef.
func reporterArtifactDoc(t *testing.T) map[string]any {
	t.Helper()

	const doc = `{
		"id": "68a0000000000000000000a1",
		"name": "reporter-artifact",
		"type": "service",
		"spec": {
			"containerGroups": [
				{
					"name": "default",
					"containers": [
						{
							"name": "app",
							"primary": true,
							"port": 8080,
							"imageUri": "registry.internal/built-by-server:abc",
							"imageOutdated": true,
							"build": {
								"artifactImageBuildId": "build-123",
								"status": "completed",
								"createdAt": "2026-07-02T10:00:00Z"
							},
							"imageBuildConfig": {
								"dockerfile": {"source": "provided"},
								"codeRef": {"datarobot": {"catalogId": "cat1", "catalogVersionId": "ver1"}}
							}
						}
					]
				}
			]
		}
	}`

	var out map[string]any

	require.NoError(t, json.Unmarshal([]byte(doc), &out))

	return out
}

// A reporter-shaped server response carries null autoscaling, a resolved bundle,
// build outputs and computed flags. The rendered file must strip all of them
// and still round-trip through parse, validate and compile.
func TestLive_RenderRoundTripReporterShaped(t *testing.T) {
	workloadID := "68b0c1d2e3f4a5b6c7d8e9f0"
	workloadDoc := reporterWorkloadDoc(t)
	artifactDoc := reporterArtifactDoc(t)

	live, err := NewLive(workloadID, workloadDoc, artifactDoc)
	require.NoError(t, err)

	draft := live.Defaults()
	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	for _, unwanted := range []string{
		"autoscaling: null",
		"artifactImageBuildId",
		"imageOutdated",
		"resolvedBundle",
		"build:",
	} {
		assert.NotContains(t, string(rendered), unwanted, "rendered manifest must not contain %s", unwanted)
	}

	assert.Contains(t, string(rendered), "imageBuildConfig")
	assert.Contains(t, string(rendered), "source: provided")
	assert.Contains(t, string(rendered), "workloadId: "+workloadID)

	parsed, err := Parse(rendered, "")
	require.NoError(t, err)
	require.NoError(t, parsed.Validate())

	compiled, err := parsed.Compile()
	require.NoError(t, err)
	assert.Equal(t, workloadID, compiled.WorkloadID)
}

// reporterShapedLive returns a Live built from the reporter-shaped fixtures so
// each standalone render-stripping test can be self-contained.
func reporterShapedLive(t *testing.T) Live {
	t.Helper()

	workloadID := "68b0c1d2e3f4a5b6c7d8e9f0"

	live, err := NewLive(workloadID, reporterWorkloadDoc(t), reporterArtifactDoc(t))
	require.NoError(t, err)

	return live
}

// Null autoscaling is echoed by the server as a placeholder but must not be
// written into the committed manifest, where it would confuse the validator.
func TestLive_RenderStripsNullAutoscaling(t *testing.T) {
	live := reporterShapedLive(t)

	draft := live.Defaults()
	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	assert.NotContains(t, string(rendered), "autoscaling: null")
}

// The server build block is a read-only output and must be stripped before the
// manifest is written, so the next deploy does not repeat a stale build id.
func TestLive_RenderStripsBuildBlock(t *testing.T) {
	live := reporterShapedLive(t)

	draft := live.Defaults()
	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	assert.NotContains(t, string(rendered), "build:")
	assert.NotContains(t, string(rendered), "artifactImageBuildId")
}

// imageOutdated is a computed server flag, not a field the user controls, so it
// must not appear in the rendered manifest.
func TestLive_RenderStripsImageOutdated(t *testing.T) {
	live := reporterShapedLive(t)

	draft := live.Defaults()
	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	assert.NotContains(t, string(rendered), "imageOutdated")
}

// resolvedBundle is a scheduling-time server output and must not be written
// into the runtime block that the user commits.
func TestLive_RenderStripsResolvedBundle(t *testing.T) {
	live := reporterShapedLive(t)

	draft := live.Defaults()
	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	assert.NotContains(t, string(rendered), "resolvedBundle")
}

// NewLive must not mutate the caller's documents, even though it strips server
// outputs and null-valued keys from its own copies.
func TestLive_DoesNotMutateInputDocs(t *testing.T) {
	workloadDoc := reporterWorkloadDoc(t)
	artifactDoc := reporterArtifactDoc(t)

	workloadBefore := deepCopyMap(workloadDoc)
	artifactBefore := deepCopyMap(artifactDoc)

	_, err := NewLive("68b0", workloadDoc, artifactDoc)
	require.NoError(t, err)

	assert.Equal(t, workloadBefore, workloadDoc)
	assert.Equal(t, artifactBefore, artifactDoc)
}

// renderedRoot parses rendered bytes and returns the document's root mapping node.
func renderedRoot(t *testing.T, rendered []byte) *yaml.Node {
	t.Helper()

	var doc yaml.Node

	require.NoError(t, yaml.Unmarshal(rendered, &doc))
	require.Len(t, doc.Content, 1)
	require.Equal(t, yaml.MappingNode, doc.Content[0].Kind)

	return doc.Content[0]
}

// mappingKeys returns the scalar keys of a mapping node in order.
func mappingKeys(node *yaml.Node) []string {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}

	keys := make([]string, 0, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		keys = append(keys, node.Content[i].Value)
	}

	return keys
}

// walkMappingNodes calls f for every mapping node under node, including node itself.
func walkMappingNodes(node *yaml.Node, f func(*yaml.Node)) {
	if node == nil {
		return
	}

	if node.Kind == yaml.MappingNode {
		f(node)
	}

	for _, child := range node.Content {
		walkMappingNodes(child, f)
	}
}

// assertNameLeadsInIdentifierMappings walks the node tree and asserts that
// every mapping which is a sequence item under a containerGroups or
// containers key leads with name — and only those mappings. Opaque mappings
// carrying a name key elsewhere (e.g. labels, environmentVars entries) keep
// their own order, so they are deliberately not asserted against. parentKey
// is the key under which node was found ("" for the root), carried across
// sequence nodes so each item remembers the key its sequence sat under.
func assertNameLeadsInIdentifierMappings(t *testing.T, node *yaml.Node, parentKey string) {
	t.Helper()

	if node == nil {
		return
	}

	// Only mappings can be identifier mappings, and only sequences carry the
	// items under a containerGroups/containers key; everything else is a leaf.
	if node.Kind == yaml.MappingNode {
		if parentKey == keyContainerGroups || parentKey == keyContainers {
			keys := mappingKeys(node)

			require.NotEmpty(t, keys)
			assert.Equal(t, keyName, keys[0],
				"identifier mapping under %s should lead with name", parentKey)
		}

		for i := 0; i+1 < len(node.Content); i += 2 {
			assertNameLeadsInIdentifierMappings(t, node.Content[i+1], node.Content[i].Value)
		}

		return
	}

	if node.Kind == yaml.SequenceNode {
		// Carry parentKey across the sequence so each item remembers the key
		// its sequence sat under (containerGroups or containers).
		for _, item := range node.Content {
			assertNameLeadsInIdentifierMappings(t, item, parentKey)
		}
	}
}

// In the rendered YAML, every container mapping lists name as its first key.
func TestLive_RenderLeadsWithNameInContainers(t *testing.T) {
	live := liveFixture(t)

	rendered, err := live.Render()
	require.NoError(t, err)

	root := renderedRoot(t, rendered)
	spec := mapValue(mapValue(root, keyArtifact), keySpec)

	for _, group := range seqItems(mapValue(spec, keyContainerGroups)) {
		for _, container := range seqItems(mapValue(group, keyContainers)) {
			keys := mappingKeys(container)
			require.NotEmpty(t, keys)
			assert.Equal(t, keyName, keys[0], "container %q should lead with name", keys[0])
		}
	}
}

// In the rendered YAML, every container group mapping lists name as its first key.
func TestLive_RenderLeadsWithNameInContainerGroups(t *testing.T) {
	live := liveFixture(t)

	rendered, err := live.Render()
	require.NoError(t, err)

	root := renderedRoot(t, rendered)

	checkGroups := func(node *yaml.Node) {
		for _, group := range seqItems(node) {
			keys := mappingKeys(group)
			require.NotEmpty(t, keys)
			assert.Equal(t, keyName, keys[0], "container group should lead with name")
		}
	}

	spec := mapValue(mapValue(root, keyArtifact), keySpec)
	checkGroups(mapValue(spec, keyContainerGroups))

	runtime := mapValue(root, keyRuntime)
	checkGroups(mapValue(runtime, keyContainerGroups))
}

// The rendered manifest's top-level mapping lists keys in the agreed order.
func TestLive_RenderTopLevelKeyOrder(t *testing.T) {
	live := liveFixture(t)

	rendered, err := live.Render()
	require.NoError(t, err)

	root := renderedRoot(t, rendered)
	keys := mappingKeys(root)

	assert.Equal(t, []string{keyWorkloadID, keyName, keyImportance, keyArtifact, keyRuntime}, keys)
}

// After Apply mutates the spec, the rendered output still leads every container
// and container group mapping with name.
func TestLive_ApplyRenderLeadsWithName(t *testing.T) {
	live := liveFixture(t)

	draft := live.Defaults()
	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	root := renderedRoot(t, rendered)
	spec := mapValue(mapValue(root, keyArtifact), keySpec)

	for _, group := range seqItems(mapValue(spec, keyContainerGroups)) {
		keys := mappingKeys(group)
		require.NotEmpty(t, keys)
		assert.Equal(t, keyName, keys[0], "container group should lead with name after Apply")

		for _, container := range seqItems(mapValue(group, keyContainers)) {
			keys := mappingKeys(container)
			require.NotEmpty(t, keys)
			assert.Equal(t, keyName, keys[0], "container should lead with name after Apply")
		}
	}
}

// Reordering moves name to the front but does not drop or duplicate any keys.
func TestLive_RenderReorderPreservesKeys(t *testing.T) {
	live := liveFixture(t)

	rendered, err := live.Render()
	require.NoError(t, err)

	root := renderedRoot(t, rendered)

	walkMappingNodes(root, func(node *yaml.Node) {
		keys := mappingKeys(node)
		seen := make(map[string]bool, len(keys))

		for _, key := range keys {
			assert.False(t, seen[key], "key %q is duplicated", key)
			seen[key] = true
		}
	})

	// Scoped name-leading: only the sequence-item mappings found under
	// containerGroups/containers keys should lead with name. Opaque mappings
	// that happen to carry a name key (labels, environmentVars entries) keep
	// their own order, so the universal "name leads wherever present"
	// assertion the unscoped hoist relied on no longer holds.
	assertNameLeadsInIdentifierMappings(t, root, "")

	// Focused check: a known container keeps its full key set, only reordering.
	var artifactDoc map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
		"name": "a",
		"type": "service",
		"spec": {"containerGroups": [{"name": "default", "containers": [
			{"name": "primary", "primary": true, "port": 8080, "imageUri": "nginx:latest"}
		]}]}
	}`), &artifactDoc))

	focused, err := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)
	require.NoError(t, err)

	focusedRendered, err := focused.Render()
	require.NoError(t, err)

	focusedRoot := renderedRoot(t, focusedRendered)
	spec := mapValue(mapValue(focusedRoot, keyArtifact), keySpec)
	group := seqItems(mapValue(spec, keyContainerGroups))[0]
	container := seqItems(mapValue(group, keyContainers))[0]

	keys := mappingKeys(container)
	assert.Equal(t, []string{keyName, keyImageURI, keyPort, keyPrimary}, keys)
}

// Opaque mappings carrying a name key (e.g. labels) are not identifier
// mappings, so name-hoisting must leave their key order alone. The
// pass-through contract (doc.go: unknown blocks "survive the round trip";
// live.go: blocks are "passed through as the platform gave them") forbids
// reordering blocks the wizard has no vocabulary for, and the unscoped
// hoist reordered every nested mapping that happened to contain a key
// literally called name — producing unexplained diff noise on every bind.
func TestLive_RenderPreservesNameInOpaqueMappings(t *testing.T) {
	var artifactDoc map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
		"name": "a",
		"type": "service",
		"spec": {"containerGroups": [{"name": "default", "containers": [
			{"name": "primary", "primary": true, "port": 8080, "imageUri": "nginx:latest",
			 "labels": {"zone": "b", "team": "x", "app": "c", "name": "not-an-id"}}
		]}]}
	}`), &artifactDoc))

	live, err := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)
	require.NoError(t, err)

	rendered, err := live.Render()
	require.NoError(t, err)

	root := renderedRoot(t, rendered)
	spec := mapValue(mapValue(root, keyArtifact), keySpec)
	group := seqItems(mapValue(spec, keyContainerGroups))[0]
	container := seqItems(mapValue(group, keyContainers))[0]

	// The container is an identifier mapping, so name still leads.
	containerKeys := mappingKeys(container)

	assert.Equal(t, keyName, containerKeys[0], "container should lead with name")

	// The labels block is opaque: name stays in its sorted position rather
	// than being hoisted to the front. The yaml encoder sorts map[string]any
	// keys alphabetically, so without hoisting name lands at its sorted
	// spot (app, name, team, zone) — hoisting is the only thing that would
	// move it to the front.
	labels := mapValue(container, "labels")
	labelsKeys := mappingKeys(labels)

	assert.Equal(t, []string{"app", "name", "team", "zone"}, labelsKeys,
		"opaque labels mapping keeps name in place, not hoisted")
}

// An artifact doc whose container carries imageUri alongside an
// imageBuildConfig with only server-managed keys (codeRef + dockerfile:null)
// is the exact review repro for RAPTOR-19533. stripKeys purges dockerfile:null
// first; stripBuildOutputs then deletes codeRef, which empties the map. The
// fix keeps the user-declared imageUri and drops the emptied imageBuildConfig
// key entirely, so the rendered file has a single valid image source and
// round-trips through Parse + Validate.
func TestLive_EmptyBuildBlockRoundTripsAfterStripping(t *testing.T) {
	var artifactDoc map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
		"name": "a",
		"type": "service",
		"spec": {"containerGroups": [{"name": "default", "containers": [
			{
				"name": "app",
				"primary": true,
				"port": 8080,
				"imageUri": "registry.example.com/app:v1",
				"imageBuildConfig": {
					"codeRef": {"datarobot": {"catalogId": "cat1", "catalogVersionId": "ver1"}},
					"dockerfile": null
				}
			}
		]}]}
	}`), &artifactDoc))

	live, err := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)
	require.NoError(t, err)

	draft := live.Defaults()
	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	// The rendered container keeps the user-declared imageUri and has no
	// imageBuildConfig key at all — not an empty mapping that Validate would
	// reject alongside imageUri.
	assert.Contains(t, string(rendered), "imageUri: registry.example.com/app:v1")
	assert.NotContains(t, string(rendered), "imageBuildConfig")
	assert.NotContains(t, string(rendered), "codeRef")

	parsed, err := Parse(rendered, "")
	require.NoError(t, err)
	require.NoError(t, parsed.Validate(), "rendered manifest must satisfy its own validator")
}

// A server doc that arrives with imageBuildConfig: {} outright (already empty,
// no dockerfile to purge) gets the same clean handling: imageUri is kept and
// the empty imageBuildConfig mapping is dropped entirely.
func TestLive_NativelyEmptyBuildBlockRoundTrips(t *testing.T) {
	var artifactDoc map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
		"name": "a",
		"type": "service",
		"spec": {"containerGroups": [{"name": "default", "containers": [
			{
				"name": "app",
				"primary": true,
				"port": 8080,
				"imageUri": "registry.example.com/app:v1",
				"imageBuildConfig": {}
			}
		]}]}
	}`), &artifactDoc))

	live, err := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)
	require.NoError(t, err)

	draft := live.Defaults()
	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	assert.Contains(t, string(rendered), "imageUri: registry.example.com/app:v1")
	assert.NotContains(t, string(rendered), "imageBuildConfig")

	parsed, err := Parse(rendered, "")
	require.NoError(t, err)
	require.NoError(t, parsed.Validate())
}

// liveBuild must classify an empty imageBuildConfig as BuildModeImage, not
// BuildModeUnchanged. An emptied build block has no content to preserve, and
// BuildModeUnchanged causes applyBuild to no-op — leaving the broken container
// in place. BuildModeImage lets applyBuild keep/restore imageUri.
//
// The "natively empty" case calls liveBuild directly. The "emptied through
// strip pipeline" case drives the real strip pipeline via NewLive: a server
// doc with imageBuildConfig {dockerfile: null, codeRef: {...}} + imageUri →
// stripKeys purges dockerfile:null → stripBuildOutputs deletes codeRef and,
// because the config is now empty, drops the imageBuildConfig key and keeps
// imageUri → the container classifies as BuildModeImage. (VAL-R3-006).
func TestLive_EmptyBuildConfigClassifiedAsImage(t *testing.T) {
	t.Run("natively empty", func(t *testing.T) {
		container := map[string]any{
			"name":             "app",
			"imageUri":         "registry.example.com/app:v1",
			"imageBuildConfig": map[string]any{},
		}

		build := liveBuild(container)

		assert.Equal(t, BuildModeImage, build.Mode,
			"empty imageBuildConfig must classify as BuildModeImage, not BuildModeUnchanged")
		assert.Equal(t, "registry.example.com/app:v1", build.ImageURI)
	})

	t.Run("emptied through strip pipeline", func(t *testing.T) {
		var artifactDoc map[string]any

		require.NoError(t, json.Unmarshal([]byte(`{
			"name": "a",
			"type": "service",
			"spec": {"containerGroups": [{"name": "default", "containers": [
				{
					"name": "app",
					"primary": true,
					"port": 8080,
					"imageUri": "registry.example.com/app:v1",
					"imageBuildConfig": {
						"dockerfile": null,
						"codeRef": {"datarobot": {"catalogId": "cat1", "catalogVersionId": "ver1"}}
					}
				}
			]}]}
		}`), &artifactDoc))

		live, err := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)
		require.NoError(t, err)

		// After NewLive: stripKeys purged dockerfile:null, stripBuildOutputs
		// deleted codeRef, and because the config is now empty, dropped the
		// imageBuildConfig key entirely and kept imageUri.
		container := primaryContainerOf(live.Spec)
		require.NotNil(t, container)

		assert.NotContains(t, container, keyImageBuildConfig,
			"the emptied imageBuildConfig key must be dropped")
		assert.Equal(t, "registry.example.com/app:v1", stringAt(container, keyImageURI),
			"the user-declared imageUri must be kept")

		// The container classifies as BuildModeImage, not BuildModeUnchanged.
		build := liveBuild(container)

		assert.Equal(t, BuildModeImage, build.Mode,
			"emptied imageBuildConfig must classify as BuildModeImage, not BuildModeUnchanged")
		assert.Equal(t, "registry.example.com/app:v1", build.ImageURI)
	})
}

// A real build container — imageBuildConfig with a dockerfile present plus a
// codeRef — still gets imageUri and codeRef stripped, and the rendered file
// keeps the build config with its dockerfile. This is the regression guard
// ensuring the empty-block fix does not weaken the real-build stripping path.
func TestLive_RealBuildContainerStillStripsImageUriAndCodeRef(t *testing.T) {
	var artifactDoc map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
		"name": "a",
		"type": "service",
		"spec": {"containerGroups": [{"name": "default", "containers": [
			{
				"name": "app",
				"primary": true,
				"port": 8080,
				"imageUri": "registry.internal/built-by-server:abc",
				"imageBuildConfig": {
					"dockerfile": {"source": "provided"},
					"codeRef": {"datarobot": {"catalogId": "cat1", "catalogVersionId": "ver1"}}
				}
			}
		]}]}
	}`), &artifactDoc))

	live, err := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)
	require.NoError(t, err)

	draft := live.Defaults()
	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	// A real build container: imageUri is stripped (server-built pin), codeRef
	// is stripped (server-managed), but the dockerfile block survives.
	assert.NotContains(t, string(rendered), "built-by-server")
	assert.NotContains(t, string(rendered), "codeRef")
	assert.Contains(t, string(rendered), "imageBuildConfig")
	assert.Contains(t, string(rendered), "source: provided")

	parsed, err := Parse(rendered, "")
	require.NoError(t, err)
	require.NoError(t, parsed.Validate())
}

// TestLive_AutoscalingEnabledValuesSurviveStripping pins the boundary that
// normalizeAutoscalingEnabled's removal must not cross: real autoscaling
// enabled values (true and false) survive NewLive/Apply/Render unchanged, and
// a null autoscaling block is still purged by the nil-purge in stripKeys.
//
// autoscaling.enabled is non-nullable on the server (bool = Field(default=True),
// protons/runtime.py:217), so enabled: null can never arrive from the server.
// This test pins the shapes that CAN arrive and must keep round-tripping
// cleanly both before and after the dead-defense removal.
func TestLive_AutoscalingEnabledValuesSurviveStripping(t *testing.T) {
	artifactJSON := `{
		"name": "a",
		"type": "service",
		"spec": {"containerGroups": [{"name": "default", "containers": [
			{"name": "app", "primary": true, "port": 8080, "imageUri": "nginx:latest"}
		]}]}
	}`

	t.Run("enabled true round-trips as ON", func(t *testing.T) {
		var workloadDoc, artifactDoc map[string]any

		require.NoError(t, json.Unmarshal([]byte(`{
			"name": "auto-on",
			"runtime": {"containerGroups": [{"name": "default",
				"autoscaling": {"enabled": true, "minReplicaCount": 1, "maxReplicaCount": 4},
				"containers": [{"name": "app", "resourceAllocation": {"cpu": 1, "memory": "1GB"}}]
			}]}
		}`), &workloadDoc))

		require.NoError(t, json.Unmarshal([]byte(artifactJSON), &artifactDoc))

		live, err := NewLive("68b0", workloadDoc, artifactDoc)
		require.NoError(t, err)

		assert.True(t, live.Autoscaled(), "enabled: true is autoscaling ON")

		draft := live.Defaults()

		assert.Equal(t, 0, draft.Runtime.Replicas, "no replica count for an autoscaled group")

		applied, err := live.Apply(draft)
		require.NoError(t, err)

		rendered, err := applied.Render()
		require.NoError(t, err)

		assert.Contains(t, string(rendered), "enabled: true")
		assert.Contains(t, string(rendered), "maxReplicaCount: 4")
		assert.NotContains(t, string(rendered), "replicaCount",
			"an autoscaled group must not carry a replica count")

		parsed, err := Parse(rendered, "")
		require.NoError(t, err)
		require.NoError(t, parsed.Validate())
	})

	t.Run("enabled false round-trips as OFF", func(t *testing.T) {
		var workloadDoc, artifactDoc map[string]any

		require.NoError(t, json.Unmarshal([]byte(`{
			"name": "auto-off",
			"runtime": {"containerGroups": [{"name": "default",
				"replicaCount": 3,
				"autoscaling": {"enabled": false, "minReplicaCount": 1, "maxReplicaCount": 4},
				"containers": [{"name": "app", "resourceAllocation": {"cpu": 1, "memory": "1GB"}}]
			}]}
		}`), &workloadDoc))

		require.NoError(t, json.Unmarshal([]byte(artifactJSON), &artifactDoc))

		live, err := NewLive("68b0", workloadDoc, artifactDoc)
		require.NoError(t, err)

		assert.False(t, live.Autoscaled(), "enabled: false is autoscaling OFF")

		draft := live.Defaults()

		assert.Equal(t, 3, draft.Runtime.Replicas, "the group's own count is reported")

		applied, err := live.Apply(draft)
		require.NoError(t, err)

		rendered, err := applied.Render()
		require.NoError(t, err)

		assert.Contains(t, string(rendered), "enabled: false")
		assert.Contains(t, string(rendered), "replicaCount: 3")

		parsed, err := Parse(rendered, "")
		require.NoError(t, err)
		require.NoError(t, parsed.Validate())
	})

	t.Run("null autoscaling block is purged", func(t *testing.T) {
		var workloadDoc, artifactDoc map[string]any

		require.NoError(t, json.Unmarshal([]byte(`{
			"name": "no-auto",
			"runtime": {"containerGroups": [{"name": "default",
				"replicaCount": 3,
				"autoscaling": null,
				"containers": [{"name": "app", "resourceAllocation": {"cpu": 1, "memory": "1GB"}}]
			}]}
		}`), &workloadDoc))

		require.NoError(t, json.Unmarshal([]byte(artifactJSON), &artifactDoc))

		live, err := NewLive("68b0", workloadDoc, artifactDoc)
		require.NoError(t, err)

		assert.False(t, live.Autoscaled(), "a null autoscaling block is absent, not ON")

		draft := live.Defaults()

		assert.Equal(t, 3, draft.Runtime.Replicas, "the group's own count is reported")

		applied, err := live.Apply(draft)
		require.NoError(t, err)

		rendered, err := applied.Render()
		require.NoError(t, err)

		assert.NotContains(t, string(rendered), "autoscaling",
			"a null autoscaling block is purged entirely by the nil-purge")

		parsed, err := Parse(rendered, "")
		require.NoError(t, err)
		require.NoError(t, parsed.Validate())
	})
}

// An artifact document carrying no usable spec is the review repro for
// finding 7: a partial or permission-filtered artifact GET returns name and
// type but no spec, and NewLive would render an inline artifact block with
// only those two fields. Validate would then reject the file the CLI just
// wrote ("artifact.spec: is required for an inline artifact") — the same
// write-then-refuse class as the titled bug. NewLive must fail loudly at bind
// time instead, naming the workload id so the user knows what to edit.
func TestNewLive_SpeclessArtifactDocFailsWithWorkloadID(t *testing.T) {
	workloadID := "68b0c1d2e3f4a5b6c7d8e9f0"

	// No spec key at all — the shape a permission-filtered GET leaves.
	artifactDoc := map[string]any{"name": "art", "type": "application"}

	_, err := NewLive(workloadID, map[string]any{"name": "wl"}, artifactDoc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), workloadID,
		"the error must name the workload id so the user knows what to edit")
	assert.Contains(t, err.Error(), "no artifact spec")
	// The message is path-neutral: it surfaces verbatim through both the
	// wizard bind flow and `dr workload up` (look.go), so it must not say
	// "to bind" — the up path is not binding anything. (VAL-R3-005)
	assert.Contains(t, err.Error(), "edit",
		"the error must tell the user to edit the file by hand")
	assert.NotContains(t, err.Error(), "bind",
		"the message must be path-neutral, not bind-specific")
}

// An artifact doc whose spec is present but strips to nothing — every key is
// server-managed or nil — is the same class of failure: after stripping, the
// spec is empty and rendering it would produce a file Validate rejects.
func TestNewLive_SpecStripsToEmptyFailsWithWorkloadID(t *testing.T) {
	workloadID := "68b0c1d2e3f4a5b6c7d8e9f0"

	// Every key inside spec is server-managed, so stripServerManaged leaves
	// an empty map.
	artifactDoc := map[string]any{
		"name": "art",
		"type": "application",
		"spec": map[string]any{
			"id":     "68a0",
			"status": "locked",
		},
	}

	_, err := NewLive(workloadID, map[string]any{"name": "wl"}, artifactDoc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), workloadID,
		"the error must name the workload id even when spec strips to empty")
	assert.Contains(t, err.Error(), "no artifact spec")
}

// An artifact doc with an explicitly empty spec mapping is the same failure:
// there is nothing to bind, and the error fires at bind time rather than at
// validate time.
func TestNewLive_EmptySpecFailsWithWorkloadID(t *testing.T) {
	workloadID := "68b0deadbeef"

	artifactDoc := map[string]any{
		"name": "art",
		"type": "service",
		"spec": map[string]any{},
	}

	_, err := NewLive(workloadID, map[string]any{"name": "wl"}, artifactDoc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), workloadID)
}

// The environmentVars path has three discriminator variants — string (source
// absent or "string" with a value), dr-credential (drCredentialId + key, no
// value), and api-key (source only, no value). Existing env-var tests feed
// hand-written YAML straight to the validator; the round trip is what proves
// the writer (Render) and the validator agree on all three shapes. The
// rejection direction — a string-variant entry with null or absent value — is
// already covered by TestValidate_EnvironmentVarsNullValueRejected and
// TestValidate_EnvironmentVarsNoValueNoSourceRejected in validate_test.go;
// the NEW coverage here is the round trip of the three valid variants.
func TestLive_RenderRoundTripsEnvironmentVarsAllVariants(t *testing.T) {
	workloadID := "68b0c1d2e3f4a5b6c7d8e9f0"

	// A service artifact whose single container carries one entry of each
	// discriminator variant, shaped exactly like a server GET response.
	artifactDoc := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(`{
      "name": "envvars-artifact",
      "spec": {"type": "service", "containerGroups": [{"name": "default", "containers": [
        {"name": "primary", "primary": true, "port": 8080,
         "imageUri": "nginx:latest",
         "environmentVars": [
           {"name": "LOG_LEVEL", "value": "debug"},
           {"name": "API_TOKEN", "source": "dr-credential",
            "drCredentialId": "68f0cccc0000000000000003", "key": "apiToken"},
           {"name": "CLOUD_KEY", "source": "api-key"}
         ]}
      ]}]}
    }`), &artifactDoc))

	workloadDoc := map[string]any{"name": "envvars-workload"}

	live, err := NewLive(workloadID, workloadDoc, artifactDoc)
	require.NoError(t, err)

	draft := live.Defaults()

	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	// Every variant must survive the render in its correct spelling. The
	// dr-credential discriminator is the only valid credential variant
	// (library/api-reference.md) — the fixture must not regress to "credential".
	for _, kept := range []string{
		"LOG_LEVEL", "debug",
		"API_TOKEN", "dr-credential", "68f0cccc0000000000000003", "apiToken",
		"CLOUD_KEY", "api-key",
	} {
		assert.Contains(t, string(rendered), kept, "render must keep %s intact", kept)
	}

	// The rendered manifest must parse and validate — the writer and the
	// validator agree on all three variant shapes.
	parsed, err := Parse(rendered, "")
	require.NoError(t, err)

	require.NoError(t, parsed.Validate())

	// Compile confirms the credential reference is syntactically valid and
	// the workload id is extractable from the rendered file.
	compiled, err := parsed.Compile()
	require.NoError(t, err)

	assert.Equal(t, workloadID, compiled.WorkloadID)
	require.Len(t, compiled.CredentialRefs, 1)
	assert.Equal(t, "68f0cccc0000000000000003", compiled.CredentialRefs[0].CredentialID)
}
