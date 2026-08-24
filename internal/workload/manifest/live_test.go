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

	return NewLive("68b0c1d2e3f4a5b6c7d8e9f0", workloadDoc, artifactDoc)
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
	live := NewLive("68b0", map[string]any{"name": "empty"}, map[string]any{"name": "empty-artifact"})

	_, err := live.Apply(live.Defaults())
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
           {"name": "OBJECT", "source": "credential",
            "drCredentialId": "68f0cccc0000000000000003", "key": "apiToken"},
           {"name": "REGION", "value": "eu-west-1"}
         ]}
      ]}]}
    }`), &artifactDoc))

	live := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)

	assert.Equal(t, []EnvVar{
		{Name: "LOG_LEVEL", Value: "debug"},
		{Name: "REGION", Value: "eu-west-1"},
	}, live.LiteralEnvVars())
}

// A spec with no container has nothing declared, which is not the same as
// having something this cannot read.
func TestLive_LiteralEnvVarsWithoutAContainer(t *testing.T) {
	live := NewLive("68b0", map[string]any{"name": "empty"}, map[string]any{"name": "empty-artifact"})

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

	live := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)

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

	live := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)

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
func TestLive_UnknownBuildSourceIsPreserved(t *testing.T) {
	var artifactDoc map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
      "name": "a",
      "type": "service", "spec": {"containerGroups": [{"name": "default", "containers": [
        {"name": "primary", "primary": true, "port": 8080,
         "imageBuildConfig": {"buildpack": {"builder": "paketo/builder:base"}}}]}]}
    }`), &artifactDoc))

	live := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)

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

	live := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)

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

	live := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)

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

	live := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)

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

	return NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)
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

	live := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)

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

	live := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)

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

	live := NewLive("68b0", workloadDoc, artifactDoc)

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

	live := NewLive("68b0", workloadDoc, artifactDoc)

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
