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
	"errors"
	"net/http"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// liveWorkloadJSON and liveArtifactJSON are shaped like the server's own
// answers, carrying the things the CLI has no vocabulary for: autoscaling, a
// GPU bundle, a sidecar, a credential-backed variable. Those are exactly what
// a plan must not mistake for drift.
const (
	liveWorkloadJSON = `{
  "id": "68b0c1d2e3f4a5b6c7d8e9f0",
  "name": "gpt-oss-20b-vllm",
  "status": "running",
  "importance": "moderate",
  "artifactId": "68a0000000000000000000a1",
  "endpoint": "https://app.datarobot.com/workloads/68b0/",
  "runtime": {
    "containerGroups": [
      {
        "name": "default",
        "resourceBundles": ["gpu.medium"],
        "autoscaling": {"enabled": true, "minReplicaCount": 1, "maxReplicaCount": 4},
        "containers": [
          {"name": "vllm-server", "resourceAllocation": {"cpu": 3, "memory": "22GB", "gpu": 1}},
          {"name": "metrics-bridge", "resourceAllocation": {"cpu": 0.5, "memory": "2GB"}}
        ]
      }
    ]
  }
}`

	liveArtifactJSON = `{
  "id": "68a0000000000000000000a1",
  "name": "gpt-oss-20b-vllm-artifact",
  "status": "locked",
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
            "readinessProbe": {"path": "/health", "port": 8000}
          },
          {"name": "metrics-bridge", "imageUri": "registry/vllm-metrics-bridge:v1"}
        ]
      }
    ]
  }
}`
)

func doc(t *testing.T, raw string) workload.Document {
	t.Helper()

	var d workload.Document

	require.NoError(t, json.Unmarshal([]byte(raw), &d))

	return d
}

// stubLive points both reads at canned documents and restores them after.
func stubLive(t *testing.T, workloadJSON, artifactJSON string) {
	t.Helper()

	prevWorkload, prevArtifact := getWorkloadDocFn, getArtifactDocFn

	getWorkloadDocFn = func(string) (workload.Document, error) { return doc(t, workloadJSON), nil }
	getArtifactDocFn = func(string) (workload.Document, error) { return doc(t, artifactJSON), nil }

	t.Cleanup(func() {
		getWorkloadDocFn, getArtifactDocFn = prevWorkload, prevArtifact
	})
}

// stubLiveErr points both reads at the same failure.
func stubLiveErr(t *testing.T, err error) {
	t.Helper()

	prevWorkload, prevArtifact := getWorkloadDocFn, getArtifactDocFn

	getWorkloadDocFn = func(string) (workload.Document, error) { return nil, err }
	getArtifactDocFn = func(string) (workload.Document, error) { return nil, err }

	t.Cleanup(func() {
		getWorkloadDocFn, getArtifactDocFn = prevWorkload, prevArtifact
	})
}

func notFoundErr() error {
	return &drapi.HTTPError{StatusCode: http.StatusNotFound, URL: "https://example.invalid"}
}

// TestLook_UnboundNeedsNoRequests: a manifest that has never deployed has
// nothing to look at, and asking the platform about an empty id would be a
// guaranteed 404 dressed up as a lookup.
func TestLook_UnboundNeedsNoRequests(t *testing.T) {
	stubLiveErr(t, errors.New("no request should have been made"))

	live, err := Look("")
	require.NoError(t, err)
	assert.Equal(t, StateUnbound, live.State)
	assert.Empty(t, live.ArtifactID)
}

func TestLook_ReadsBothDocuments(t *testing.T) {
	stubLive(t, liveWorkloadJSON, liveArtifactJSON)

	live, err := Look("68b0c1d2e3f4a5b6c7d8e9f0")
	require.NoError(t, err)

	assert.Equal(t, StateRunning, live.State)
	assert.Equal(t, "gpt-oss-20b-vllm", live.Name)
	assert.Equal(t, "moderate", live.Importance)
	assert.Equal(t, "68a0000000000000000000a1", live.ArtifactID)
	assert.Equal(t, "gpt-oss-20b-vllm-artifact", live.ArtifactName)
	assert.Equal(t, "https://app.datarobot.com/workloads/68b0/", live.Endpoint)
	assert.True(t, live.Locked)
	assert.NotEmpty(t, live.Spec)
	assert.NotEmpty(t, live.Runtime)
}

// TestLook_StripsBuildOutputsFromTheSpec is the property that keeps a
// server-built image from reading as drift forever. The container says how to
// build itself, so the image the server produced and the code ref it produced
// it from are both the server's business, not the file's.
func TestLook_StripsBuildOutputsFromTheSpec(t *testing.T) {
	stubLive(t, liveWorkloadJSON, liveArtifactJSON)

	live, err := Look("wl-1")
	require.NoError(t, err)

	primary := primaryContainer(t, live.Spec)
	assert.NotContains(t, primary, "imageUri", "the built image belongs to the server, not the manifest")

	build, _ := primary["imageBuildConfig"].(map[string]any)
	assert.NotContains(t, build, "codeRef")
	assert.Contains(t, build, "dockerfile", "how to build is the file's business and must survive")
}

// TestLook_SpecSurvivesUnknownBlocks: the sidecar has no counterpart in
// anything the CLI can express, and it still has to come through, because
// Subset compares against it and a plan that dropped it would propose
// deleting a running container.
func TestLook_SpecSurvivesUnknownBlocks(t *testing.T) {
	stubLive(t, liveWorkloadJSON, liveArtifactJSON)

	live, err := Look("wl-1")
	require.NoError(t, err)

	groups, _ := live.Spec["containerGroups"].([]any)
	require.Len(t, groups, 1)

	group, _ := groups[0].(map[string]any)
	containers, _ := group["containers"].([]any)
	assert.Len(t, containers, 2, "the sidecar has to survive the trip")

	runtimeGroups, _ := live.Runtime["containerGroups"].([]any)
	require.Len(t, runtimeGroups, 1)

	runtimeGroup, _ := runtimeGroups[0].(map[string]any)
	assert.Contains(t, runtimeGroup, "resourceBundles")
	assert.Contains(t, runtimeGroup, "autoscaling")
}

// TestLook_MissingWorkloadIsAStateNotAnError: whether a vanished workload is
// fatal depends on what the caller is allowed to do about it, which this
// layer does not know.
func TestLook_MissingWorkloadIsAStateNotAnError(t *testing.T) {
	stubLiveErr(t, notFoundErr())

	live, err := Look("wl-gone")
	require.NoError(t, err)
	assert.Equal(t, StateMissing, live.State)
	assert.Empty(t, live.Endpoint)
}

func TestLook_TransportFailurePropagates(t *testing.T) {
	stubLiveErr(t, &drapi.HTTPError{StatusCode: http.StatusInternalServerError})

	_, err := Look("wl-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot read workload wl-1")
}

// TestLook_MissingArtifactIsNotFatal covers the window around creation where
// a workload exists but its artifact does not answer yet. An absent spec
// makes every field the file names read as an addition, which is the right
// description of something not built.
func TestLook_MissingArtifactIsNotFatal(t *testing.T) {
	prevWorkload, prevArtifact := getWorkloadDocFn, getArtifactDocFn

	getWorkloadDocFn = func(string) (workload.Document, error) { return doc(t, liveWorkloadJSON), nil }
	getArtifactDocFn = func(string) (workload.Document, error) { return nil, notFoundErr() }

	t.Cleanup(func() { getWorkloadDocFn, getArtifactDocFn = prevWorkload, prevArtifact })

	live, err := Look("wl-1")
	require.NoError(t, err)
	assert.Equal(t, StateRunning, live.State)
	assert.Empty(t, live.Spec)
	assert.NotEmpty(t, live.Runtime, "the runtime half lives on the workload and is unaffected")
	assert.False(t, live.Locked)
}

func TestLook_ArtifactFailurePropagates(t *testing.T) {
	prevWorkload, prevArtifact := getWorkloadDocFn, getArtifactDocFn

	getWorkloadDocFn = func(string) (workload.Document, error) { return doc(t, liveWorkloadJSON), nil }
	getArtifactDocFn = func(string) (workload.Document, error) {
		return nil, &drapi.HTTPError{StatusCode: http.StatusBadGateway}
	}

	t.Cleanup(func() { getWorkloadDocFn, getArtifactDocFn = prevWorkload, prevArtifact })

	_, err := Look("wl-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot read artifact")
}

func TestLook_WorkloadWithNoArtifactSkipsTheSecondRead(t *testing.T) {
	var artifactReads int

	prevWorkload, prevArtifact := getWorkloadDocFn, getArtifactDocFn

	getWorkloadDocFn = func(string) (workload.Document, error) {
		return doc(t, `{"id":"wl-1","name":"fresh","status":"submitted"}`), nil
	}
	getArtifactDocFn = func(string) (workload.Document, error) {
		artifactReads++

		return nil, nil
	}

	t.Cleanup(func() { getWorkloadDocFn, getArtifactDocFn = prevWorkload, prevArtifact })

	live, err := Look("wl-1")
	require.NoError(t, err)
	assert.Equal(t, StateSettling, live.State)
	assert.Zero(t, artifactReads, "there is no artifact id to look up")
}

// TestLook_LockedIsCaseInsensitive: the platform's docs disagree with
// themselves on status casing, and reading a locked artifact as a draft would
// let a rollout be attempted that the platform will refuse.
func TestLook_LockedIsCaseInsensitive(t *testing.T) {
	for _, status := range []string{"locked", "LOCKED", "Locked"} {
		t.Run(status, func(t *testing.T) {
			stubLive(t, liveWorkloadJSON, `{"id":"art-1","status":"`+status+`","spec":{"type":"service"}}`)

			live, err := Look("wl-1")
			require.NoError(t, err)
			assert.True(t, live.Locked)
		})
	}

	stubLive(t, liveWorkloadJSON, `{"id":"art-1","status":"draft","spec":{"type":"service"}}`)

	live, err := Look("wl-1")
	require.NoError(t, err)
	assert.False(t, live.Locked)
}

// TestLook_SpeclessArtifactFailsLoudly pins the deliberate behavior that a
// present-but-spec-less artifact document fails the up path rather than
// rendering a manifest Validate would reject. The error from NewLive names the
// workload id. Empty artifactID and 404 from artifactFor remain exempt:
// artifactDoc is nil, so the error is swallowed and Look returns a Live with
// an empty spec and no error.
func TestLook_SpeclessArtifactFailsLoudly(t *testing.T) {
	t.Run("spec absent", func(t *testing.T) {
		stubLive(t, liveWorkloadJSON, `{"id":"art-1","status":"locked"}`)

		_, err := Look("wl-specless")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "wl-specless", "error must name the workload id")
		assert.Contains(t, err.Error(), "no artifact spec")
	})

	t.Run("spec null", func(t *testing.T) {
		stubLive(t, liveWorkloadJSON, `{"id":"art-1","status":"locked","spec":null}`)

		_, err := Look("wl-specless")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "wl-specless")
	})

	t.Run("spec empty mapping", func(t *testing.T) {
		stubLive(t, liveWorkloadJSON, `{"id":"art-1","status":"locked","spec":{}}`)

		_, err := Look("wl-specless")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "wl-specless")
	})

	t.Run("spec strips to nothing", func(t *testing.T) {
		// spec carries only server-managed keys that stripServerManaged removes
		stubLive(t, liveWorkloadJSON, `{"id":"art-1","status":"locked","spec":{"id":"x","createdAt":"now","status":"ok"}}`)

		_, err := Look("wl-specless")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "wl-specless")
	})

	t.Run("empty artifactId exempt", func(t *testing.T) {
		prev := getWorkloadDocFn

		getWorkloadDocFn = func(string) (workload.Document, error) {
			return doc(t, `{"id":"wl-1","name":"fresh","status":"submitted"}`), nil
		}

		t.Cleanup(func() { getWorkloadDocFn = prev })

		live, err := Look("wl-1")
		require.NoError(t, err)
		assert.Empty(t, live.Spec, "no artifact to read means no spec, not an error")
	})

	t.Run("artifact 404 exempt", func(t *testing.T) {
		prevWorkload, prevArtifact := getWorkloadDocFn, getArtifactDocFn

		getWorkloadDocFn = func(string) (workload.Document, error) { return doc(t, liveWorkloadJSON), nil }
		getArtifactDocFn = func(string) (workload.Document, error) { return nil, notFoundErr() }

		t.Cleanup(func() { getWorkloadDocFn, getArtifactDocFn = prevWorkload, prevArtifact })

		live, err := Look("wl-1")
		require.NoError(t, err)
		assert.Empty(t, live.Spec, "a vanished artifact is not a failure")
	})
}

// primaryContainer digs the flagged container out of a live spec.
func primaryContainer(t *testing.T, spec map[string]any) map[string]any {
	t.Helper()

	groups, _ := spec["containerGroups"].([]any)
	require.NotEmpty(t, groups)

	group, _ := groups[0].(map[string]any)
	containers, _ := group["containers"].([]any)
	require.NotEmpty(t, containers)

	container, _ := containers[0].(map[string]any)

	return container
}
