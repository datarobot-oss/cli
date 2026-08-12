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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// obj decodes a JSON literal the way both sides of a real comparison arrive:
// the manifest through Compile's payload, the live object through the API.
// Going via encoding/json in the test too is the point, since it is what
// makes every number a float64 on both sides.
func obj(t *testing.T, raw string) map[string]any {
	t.Helper()

	var m map[string]any

	require.NoError(t, json.Unmarshal([]byte(raw), &m))

	return m
}

// paths is the set of changed paths, which is what most cases care about.
func paths(changes []Change) []string {
	out := make([]string, 0, len(changes))
	for _, c := range changes {
		out = append(out, c.Path)
	}

	return out
}

func TestSubset_IdenticalIsNoDrift(t *testing.T) {
	same := `{"type":"service","port":8080,"probe":{"path":"/health"}}`

	assert.Empty(t, Subset(obj(t, same), obj(t, same)))
}

func TestSubset_ScalarDifference(t *testing.T) {
	changes := Subset(
		obj(t, `{"port":9090}`),
		obj(t, `{"port":8080}`),
	)

	require.Len(t, changes, 1)
	assert.Equal(t, "port", changes[0].Path)
	assert.InDelta(t, 9090.0, changes[0].Want, 0)
	assert.InDelta(t, 8080.0, changes[0].Have, 0)
	assert.False(t, changes[0].Absent)
}

func TestSubset_MissingLiveFieldIsAnAddition(t *testing.T) {
	changes := Subset(
		obj(t, `{"readinessProbe":{"path":"/health"}}`),
		obj(t, `{}`),
	)

	require.Len(t, changes, 1)
	assert.Equal(t, "readinessProbe", changes[0].Path)
	assert.True(t, changes[0].Absent, "a field the live object never had is an addition, not a change")
	assert.Nil(t, changes[0].Have)
}

// TestSubset_LiveExtrasAreNotDrift is the contract the whole command rests
// on. A workload can carry a GPU bundle, a sidecar and an autoscaling policy
// the manifest has never heard of, and none of that is a reason to redeploy.
func TestSubset_LiveExtrasAreNotDrift(t *testing.T) {
	changes := Subset(
		obj(t, `{"type":"service"}`),
		obj(t, `{"type":"service","resourceBundles":["gpu.medium"],"storage":{"mode":"nimCache"}}`),
	)

	assert.Empty(t, changes)
}

// TestSubset_ExplicitNullLiveIsAChangeNotAnAddition separates "the key is
// there holding null" from "the key is not there", which walk tracks with
// its present flag. Both render differently in a plan.
func TestSubset_ExplicitNullLiveIsAChangeNotAnAddition(t *testing.T) {
	changes := Subset(
		obj(t, `{"entrypoint":["uvicorn"]}`),
		obj(t, `{"entrypoint":null}`),
	)

	require.Len(t, changes, 1)
	assert.False(t, changes[0].Absent)
	assert.Nil(t, changes[0].Have)
}

// TestSubset_NameKeyedListsMatchByNameNotIndex is why walkList exists. The
// platform returns container groups in whatever order it likes; matching by
// index would report a reordered live object as a wholesale rewrite and
// trigger a rollout that changes nothing.
func TestSubset_NameKeyedListsMatchByNameNotIndex(t *testing.T) {
	want := obj(t, `{"containers":[
		{"name":"primary","port":8080},
		{"name":"sidecar","port":9000}
	]}`)
	have := obj(t, `{"containers":[
		{"name":"sidecar","port":9000},
		{"name":"primary","port":8080}
	]}`)

	assert.Empty(t, Subset(want, have), "reordering a name-keyed list is not drift")
}

func TestSubset_NameKeyedPathUsesTheName(t *testing.T) {
	changes := Subset(
		obj(t, `{"containerGroups":[{"name":"default","containers":[{"name":"primary","port":9090}]}]}`),
		obj(t, `{"containerGroups":[{"name":"default","containers":[{"name":"primary","port":8080}]}]}`),
	)

	assert.Equal(t, []string{"containerGroups[default].containers[primary].port"}, paths(changes))
}

func TestSubset_NameKeyedElementMissingLiveIsOneAddition(t *testing.T) {
	changes := Subset(
		obj(t, `{"containers":[{"name":"primary","port":8080},{"name":"sidecar","port":9000}]}`),
		obj(t, `{"containers":[{"name":"primary","port":8080}]}`),
	)

	require.Len(t, changes, 1, "a whole new container is one addition, not one per field")
	assert.Equal(t, "containers[sidecar]", changes[0].Path)
	assert.True(t, changes[0].Absent)
}

// TestSubset_UnkeyedListsCompareWhole covers resourceBundles (plain strings)
// and autoscaling policies (objects with no name). Neither has anything to
// key on, so a partial match would be meaningless.
func TestSubset_UnkeyedListsCompareWhole(t *testing.T) {
	unchanged := Subset(
		obj(t, `{"resourceBundles":["gpu.medium"]}`),
		obj(t, `{"resourceBundles":["gpu.medium"]}`),
	)
	assert.Empty(t, unchanged)

	changed := Subset(
		obj(t, `{"resourceBundles":["gpu.large"]}`),
		obj(t, `{"resourceBundles":["gpu.medium"]}`),
	)
	assert.Equal(t, []string{"resourceBundles"}, paths(changed))

	policies := Subset(
		obj(t, `{"policies":[{"scalingMetric":"gpuCacheUtilization","target":80}]}`),
		obj(t, `{"policies":[{"scalingMetric":"gpuCacheUtilization","target":70}]}`),
	)
	assert.Equal(t, []string{"policies"}, paths(policies))
}

func TestSubset_EmptyListsAreCompatible(t *testing.T) {
	assert.Empty(t, Subset(obj(t, `{"containers":[]}`), obj(t, `{"containers":[]}`)))

	changes := Subset(obj(t, `{"containers":[]}`), obj(t, `{"containers":[{"name":"primary"}]}`))
	assert.Equal(t, []string{"containers"}, paths(changes),
		"an empty list has nothing to key on, so it is compared whole")
}

func TestSubset_TypeMismatchIsOneChange(t *testing.T) {
	changes := Subset(
		obj(t, `{"probe":{"path":"/health"}}`),
		obj(t, `{"probe":"/health"}`),
	)

	require.Len(t, changes, 1)
	assert.Equal(t, "probe", changes[0].Path)

	listVsScalar := Subset(
		obj(t, `{"entrypoint":["uvicorn","app:app"]}`),
		obj(t, `{"entrypoint":"uvicorn app:app"}`),
	)
	assert.Equal(t, []string{"entrypoint"}, paths(listVsScalar))
}

// TestSubset_OrderIsStable keeps a plan from reading differently on two runs
// over the same inputs, which would make a diffed CI log useless.
func TestSubset_OrderIsStable(t *testing.T) {
	want := obj(t, `{"zebra":1,"alpha":2,"middle":3,"beta":4}`)
	have := obj(t, `{"zebra":0,"alpha":0,"middle":0,"beta":0}`)

	expected := []string{"alpha", "beta", "middle", "zebra"}

	for range 5 {
		assert.Equal(t, expected, paths(Subset(want, have)))
	}
}

// liveSpecJSON is the artifact spec as manifest.NewLive hands it over: server
// keys stripped, and crucially no imageUri or codeRef on the container that
// says how to build itself, so a server-built image can never read as drift.
const liveSpecJSON = `{
  "type": "service",
  "containerGroups": [
    {
      "name": "default",
      "containers": [
        {
          "name": "vllm-server",
          "primary": true,
          "port": 8000,
          "imageBuildConfig": {"dockerfile": {"source": "provided"}},
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
}`

// TestSubset_RealSpecUnchanged runs a manifest that says less than the live
// object against that object. The sidecar, the second probe and the extra
// probe timings are all things the file never mentions, and every one of
// them must stay invisible.
func TestSubset_RealSpecUnchanged(t *testing.T) {
	file := obj(t, `{
	  "type": "service",
	  "containerGroups": [
	    {
	      "name": "default",
	      "containers": [
	        {
	          "name": "vllm-server",
	          "primary": true,
	          "port": 8000,
	          "imageBuildConfig": {"dockerfile": {"source": "provided"}},
	          "readinessProbe": {"path": "/health", "port": 8000}
	        }
	      ]
	    }
	  ]
	}`)

	assert.Empty(t, Subset(file, obj(t, liveSpecJSON)))
}

// TestSubset_RealSpecDrift changes exactly two things and expects exactly two
// findings, addressed by name rather than by position.
func TestSubset_RealSpecDrift(t *testing.T) {
	file := obj(t, `{
	  "type": "service",
	  "containerGroups": [
	    {
	      "name": "default",
	      "containers": [
	        {
	          "name": "vllm-server",
	          "port": 8080,
	          "readinessProbe": {"path": "/ready", "port": 8000}
	        }
	      ]
	    }
	  ]
	}`)

	assert.Equal(t, []string{
		"containerGroups[default].containers[vllm-server].port",
		"containerGroups[default].containers[vllm-server].readinessProbe.path",
	}, paths(Subset(file, obj(t, liveSpecJSON))))
}

// TestSubset_CredentialRefComparesAsAnObject checks the compiled form of the
// dr-credential shorthand round-trips against what the platform stores. The
// manifest is compiled before it gets here, so both sides are the expanded
// object and the secret's value is nowhere near the comparison.
func TestSubset_CredentialRefComparesAsAnObject(t *testing.T) {
	file := obj(t, `{"containers":[{"name":"vllm-server","environmentVars":[
		{"name":"HUGGING_FACE_HUB_TOKEN","source":"dr-credential",
		 "drCredentialId":"66f1a2b3c4d5e6f7a8b9c0d1","key":"apiToken"}
	]}]}`)
	live := obj(t, `{"containers":[{"name":"vllm-server","environmentVars":[
		{"name":"HF_HOME","value":"/tmp/hf"},
		{"name":"HUGGING_FACE_HUB_TOKEN","source":"dr-credential",
		 "drCredentialId":"66f1a2b3c4d5e6f7a8b9c0d1","key":"apiToken"}
	]}]}`)

	assert.Empty(t, Subset(file, live),
		"a variable the file does not mention, and a matching credential ref, are both quiet")

	rotated := obj(t, `{"containers":[{"name":"vllm-server","environmentVars":[
		{"name":"HUGGING_FACE_HUB_TOKEN","source":"dr-credential",
		 "drCredentialId":"aaaaaaaaaaaaaaaaaaaaaaaa","key":"apiToken"}
	]}]}`)

	assert.Equal(t,
		[]string{"containers[vllm-server].environmentVars[HUGGING_FACE_HUB_TOKEN].drCredentialId"},
		paths(Subset(rotated, live)))
}

func TestSubset_RuntimeHalf(t *testing.T) {
	live := obj(t, `{"containerGroups":[{
		"name":"default",
		"resourceBundles":["gpu.medium"],
		"autoscaling":{"enabled":true,"minReplicaCount":1,"maxReplicaCount":4},
		"containers":[
			{"name":"vllm-server","resourceAllocation":{"cpu":3,"memory":"22GB","gpu":1}},
			{"name":"metrics-bridge","resourceAllocation":{"cpu":0.5,"memory":"2GB"}}
		]
	}]}`)

	file := obj(t, `{"containerGroups":[{
		"name":"default",
		"containers":[{"name":"vllm-server","resourceAllocation":{"cpu":4,"memory":"22GB"}}]
	}]}`)

	assert.Equal(t,
		[]string{"containerGroups[default].containers[vllm-server].resourceAllocation.cpu"},
		paths(Subset(file, live)),
		"the bundle, the autoscaling block and the sidecar are all unmentioned, so all quiet")
}

func TestChange_String(t *testing.T) {
	assert.Equal(t, "runtime.replicaCount: 1 -> 3",
		Change{Path: "runtime.replicaCount", Have: 1, Want: 3}.String())

	assert.Equal(t, "readinessProbe: {path, port}",
		Change{Path: "readinessProbe", Absent: true, Want: map[string]any{"path": "/health", "port": 8080}}.String())

	assert.Equal(t, "importance: unset -> low",
		Change{Path: "importance", Have: nil, Want: "low"}.String())

	assert.Equal(t, "entrypoint: [1 item(s)] -> [2 item(s)]",
		Change{Path: "entrypoint", Have: []any{"a"}, Want: []any{"a", "b"}}.String())
}
