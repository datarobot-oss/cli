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
      "spec": {"type": "service", "containerGroups": [{"name": "default", "containers": [
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
      "spec": {"type": "service", "containerGroups": [{"name": "default", "containers": [
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
// the wizard's default one, which its container would never pass.
func TestLive_ApplyDoesNotInventAReadinessProbe(t *testing.T) {
	var artifactDoc map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
      "name": "a",
      "spec": {"type": "service", "containerGroups": [{"name": "default", "containers": [
        {"name": "primary", "primary": true, "port": 9000, "imageUri": "registry/a:v1"}]}]}
    }`), &artifactDoc))

	live := NewLive("68b0", map[string]any{"name": "a"}, artifactDoc)

	applied, err := live.Apply(live.Defaults())
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	assert.NotContains(t, string(rendered), "readinessProbe")

	// One that does have a probe keeps it pointing where the answers say.
	draft := live.Defaults()
	draft.HealthPath = "/ready"

	applied, err = live.Apply(draft)
	require.NoError(t, err)
	rendered, err = applied.Render()
	require.NoError(t, err)
	assert.NotContains(t, string(rendered), "/ready", "no probe means nothing to point")
}

// A spec that flags no primary container still has one traffic reaches, and
// the file must say so or the ledger rejects the port it just wrote.
func TestLive_ApplyFlagsTheContainerItEdits(t *testing.T) {
	var artifactDoc map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
      "name": "a",
      "spec": {"type": "service", "containerGroups": [{"name": "default", "containers": [
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
