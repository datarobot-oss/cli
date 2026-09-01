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

// Extra is the half Subset deliberately leaves out: what the live side
// carries that the file no longer names.
func TestExtra_FindsWhatTheFileNoLongerNames(t *testing.T) {
	tests := []struct {
		name string
		want string
		have string
		out  []string
	}{
		{
			name: "an env var the file dropped",
			want: `{"spec":{"containerGroups":[{"name":"default","containers":[{"name":"primary"}]}]}}`,
			have: `{"spec":{"containerGroups":[{"name":"default","containers":[
			        {"name":"primary","environmentVars":[{"name":"FOO","value":"1"}]}]}]}}`,
			out: []string{"spec.containerGroups[default].containers[primary].environmentVars[FOO]"},
		},
		{
			name: "the last env var, so the file has no block at all",
			want: `{"spec":{"containerGroups":[{"name":"default","containers":[{"name":"primary","port":8080}]}]}}`,
			have: `{"spec":{"containerGroups":[{"name":"default","containers":[
			        {"name":"primary","port":8080,"environmentVars":[{"name":"FOO","value":"1"}]}]}]}}`,
			out: []string{"spec.containerGroups[default].containers[primary].environmentVars[FOO]"},
		},
		{
			name: "a sidecar the file dropped",
			want: `{"spec":{"containerGroups":[{"name":"default","containers":[{"name":"primary"}]}]}}`,
			have: `{"spec":{"containerGroups":[{"name":"default","containers":[
			        {"name":"primary"},{"name":"metrics"}]}]}}`,
			out: []string{"spec.containerGroups[default].containers[metrics]"},
		},
		{
			name: "nothing removed, so nothing to report",
			want: `{"spec":{"containerGroups":[{"name":"default","containers":[{"name":"primary","port":8080}]}]}}`,
			have: `{"spec":{"containerGroups":[{"name":"default","containers":[{"name":"primary","port":8080}]}]}}`,
			out:  nil,
		},
		{
			name: "server-owned fields are not removals",
			want: `{"spec":{"containerGroups":[{"name":"default","containers":[{"name":"primary"}]}]}}`,
			have: `{"id":"art-1","status":"draft","spec":{"containerGroups":[{"name":"default","containers":[
			        {"name":"primary","imageUri":"registry/built:sha","readinessProbe":{"path":"/health"}}]}]}}`,
			out: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var want, have map[string]any

			require.NoError(t, json.Unmarshal([]byte(tc.want), &want))
			require.NoError(t, json.Unmarshal([]byte(tc.have), &have))

			assert.Equal(t, tc.out, Extra(want, have))
		})
	}
}

// The two questions are not the same one asked twice: a file that adds
// something is Subset's business and a file that removes something is Extra's,
// and each is silent about the other.
func TestExtra_AndSubsetAnswerDifferentQuestions(t *testing.T) {
	var want, have map[string]any

	require.NoError(t, json.Unmarshal([]byte(
		`{"spec":{"containerGroups":[{"name":"default","containers":[{"name":"primary","port":9000}]}]}}`), &want))
	require.NoError(t, json.Unmarshal([]byte(
		`{"spec":{"containerGroups":[{"name":"default","containers":[
		  {"name":"primary","port":8080,"environmentVars":[{"name":"FOO","value":"1"}]}]}]}}`), &have))

	assert.Len(t, Subset(want, have), 1, "the port the file moved")
	assert.Len(t, Extra(want, have), 1, "the variable the file dropped")
}

// The platform stores a size as bytes and returns it that way, so "512MB" in
// the file and 512000000 in the live workload are one sizing written twice.
// Reporting that as drift meant a workload could never be up to date: every
// run rebuilt and rolled a project nobody had touched.
func TestSubset_MemoryIsTheSameSizeWrittenTwoWays(t *testing.T) {
	want := map[string]any{
		"containerGroups": []any{map[string]any{
			"name": "default",
			"containers": []any{map[string]any{
				"name":               "primary",
				"resourceAllocation": map[string]any{"cpu": 0.5, "memory": "512MB"},
			}},
		}},
	}
	have := map[string]any{
		"containerGroups": []any{map[string]any{
			"name": "default",
			"containers": []any{map[string]any{
				"name":               "primary",
				"resourceAllocation": map[string]any{"cpu": 0.5, "memory": 512000000.0},
			}},
		}},
	}

	assert.Empty(t, Subset(want, have), "512MB and 512000000 are the same sizing")
}

func TestSubset_ADifferentSizeIsStillDrift(t *testing.T) {
	want := map[string]any{"resourceAllocation": map[string]any{"memory": "1GB"}}
	have := map[string]any{"resourceAllocation": map[string]any{"memory": 512000000.0}}

	assert.Len(t, Subset(want, have), 1, "1GB is not 512MB")
}

// The loose comparison is keyed on the field name, so a numeric string
// elsewhere does not quietly compare equal to its number.
func TestSubset_OnlyMemoryComparesAcrossTypes(t *testing.T) {
	want := map[string]any{"port": "8080"}
	have := map[string]any{"port": 8080.0}

	assert.Len(t, Subset(want, have), 1, "a port written as a string is still a difference")
}

// Siblings must not share a backing array: appending in place would have the
// second overwrite the first's last segment, so a changed port would read as a
// changed environment variable.
func TestSubset_SiblingKeysDoNotShareStorage(t *testing.T) {
	spec := func(port, cpu float64) map[string]any {
		return map[string]any{"containerGroups": []any{map[string]any{
			"name":       "default",
			"containers": []any{map[string]any{"name": "primary", "port": port, "cpu": cpu}},
		}}}
	}

	changes := Subset(spec(9000, 2), spec(8000, 1))
	require.Len(t, changes, 2)

	last := map[string]bool{}
	for _, c := range changes {
		last[c.Keys[len(c.Keys)-1]] = true
	}

	assert.Equal(t, map[string]bool{"port": true, "cpu": true}, last,
		"each sibling keeps its own last segment")
}

// rowPaths is the set of walked paths, which is what most DiffRows cases
// care about.
func rowPaths(rows []DiffRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Path)
	}

	return out
}

// changedRows keeps the rows the two sides disagree about, which is exactly
// the set Subset reports. DiffRows is Subset's walk with the agreeing leaves
// kept, so the two must answer the same question with the same records.
func changedRows(rows []DiffRow) []DiffRow {
	out := make([]DiffRow, 0, len(rows))
	for _, r := range rows {
		if r.Changed {
			out = append(out, r)
		}
	}

	return out
}

// assertSameFindings pins the classification to Subset's: for the same two
// sides, the changed rows are the changes, one for one, values and all. A
// diff that disagreed with the plan about what differs would be worse than
// no diff at all.
func assertSameFindings(t *testing.T, want, have map[string]any) {
	t.Helper()

	changes := Subset(want, have)
	rows := changedRows(DiffRows(want, have))

	require.Len(t, rows, len(changes))

	for i, c := range changes {
		assert.Equal(t, c.Path, rows[i].Path, "row %d path", i)
		assert.Equal(t, c.Want, rows[i].Want, "row %d want", i)
		assert.Equal(t, c.Have, rows[i].Have, "row %d have", i)
		assert.Equal(t, c.Absent, rows[i].Absent, "row %d absent", i)
	}
}

// TestDiffRows_EmitsOneRowPerLeafChangedOrUnchanged is the difference from
// Subset: a diff needs the leaves that already agree as context, so every
// leaf of want gets a row, in sorted order, marked.
func TestDiffRows_EmitsOneRowPerLeafChangedOrUnchanged(t *testing.T) {
	rows := DiffRows(
		obj(t, `{"type":"service","port":9090,"probe":{"path":"/health","port":8000}}`),
		obj(t, `{"type":"service","port":8080,"probe":{"path":"/health","port":8000}}`),
	)

	require.Len(t, rows, 4, "one row per leaf of want, not one per difference")
	assert.Equal(t, []string{"port", "probe.path", "probe.port", "type"}, rowPaths(rows))

	changed := rows[0]

	assert.True(t, changed.Changed)
	assert.False(t, changed.Absent)
	assert.InDelta(t, 9090.0, changed.Want, 0)
	assert.InDelta(t, 8080.0, changed.Have, 0)

	for _, r := range rows[1:] {
		assert.False(t, r.Changed, "leaf %s agrees, so it is context", r.Path)
	}
}

// TestDiffRows_ChangedRowsAgreeWithSubset runs the fixtures Subset's own
// tests use through both walkers: whatever Subset reports, DiffRows must
// mark changed, with the same path, values and absent flag.
func TestDiffRows_ChangedRowsAgreeWithSubset(t *testing.T) {
	tests := []struct {
		name string
		want string
		have string
	}{
		{
			name: "a moved scalar",
			want: `{"port":9090}`,
			have: `{"port":8080}`,
		},
		{
			name: "a field the live object lacks",
			want: `{"readinessProbe":{"path":"/health"}}`,
			have: `{}`,
		},
		{
			name: "an explicit null on the live side",
			want: `{"entrypoint":["uvicorn"]}`,
			have: `{"entrypoint":null}`,
		},
		{
			name: "a port inside a name-keyed list",
			want: `{"containerGroups":[{"name":"default","containers":[{"name":"primary","port":9090}]}]}`,
			have: `{"containerGroups":[{"name":"default","containers":[{"name":"primary","port":8080}]}]}`,
		},
		{
			name: "a whole element the live object lacks",
			want: `{"containers":[{"name":"primary","port":8080},{"name":"sidecar","port":9000}]}`,
			have: `{"containers":[{"name":"primary","port":8080}]}`,
		},
		{
			name: "an unkeyed list that differs",
			want: `{"resourceBundles":["gpu.large"]}`,
			have: `{"resourceBundles":["gpu.medium"]}`,
		},
		{
			name: "a type mismatch",
			want: `{"probe":{"path":"/health"}}`,
			have: `{"probe":"/health"}`,
		},
		{
			name: "a genuinely different memory size",
			want: `{"resourceAllocation":{"memory":"1GB"}}`,
			have: `{"resourceAllocation":{"memory":512000000.0}}`,
		},
		{
			name: "a rotated credential reference",
			want: `{"containers":[{"name":"vllm-server","environmentVars":[
				{"name":"HUGGING_FACE_HUB_TOKEN","source":"dr-credential",
				 "drCredentialId":"aaaaaaaaaaaaaaaaaaaaaaaa","key":"apiToken"}]}]}`,
			have: `{"containers":[{"name":"vllm-server","environmentVars":[
				{"name":"HF_HOME","value":"/tmp/hf"},
				{"name":"HUGGING_FACE_HUB_TOKEN","source":"dr-credential",
				 "drCredentialId":"66f1a2b3c4d5e6f7a8b9c0d1","key":"apiToken"}]}]}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertSameFindings(t, obj(t, tc.want), obj(t, tc.have))
		})
	}

	// And the real spec, where the file moved two things and addressed them
	// by name rather than by position.
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

	assertSameFindings(t, file, obj(t, liveSpecJSON))

	// A file that says less than the live object: every leaf it does name
	// agrees, so the diff is all context and Subset is silent.
	quiet := obj(t, `{
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

	assert.Empty(t, Subset(quiet, obj(t, liveSpecJSON)))
	assert.Empty(t, changedRows(DiffRows(quiet, obj(t, liveSpecJSON))),
		"nothing the file names differs, so the diff has no changes to show")
}

// TestDiffRows_NameKeyedListsMatchByNameNotIndex is why the walk matches by
// name: the platform returns container groups in whatever order it likes,
// and a reordered live object must move no row and mark none changed.
func TestDiffRows_NameKeyedListsMatchByNameNotIndex(t *testing.T) {
	want := obj(t, `{"containers":[
		{"name":"primary","port":8080},
		{"name":"sidecar","port":9000}
	]}`)
	have := obj(t, `{"containers":[
		{"name":"sidecar","port":9000},
		{"name":"primary","port":8080}
	]}`)
	sorted := obj(t, `{"containers":[
		{"name":"primary","port":8080},
		{"name":"sidecar","port":9000}
	]}`)

	reordered := DiffRows(want, have)

	assert.Equal(t, DiffRows(want, sorted), reordered,
		"the live order of a name-keyed list moves no row")

	for _, r := range reordered {
		assert.False(t, r.Changed, "reordering a name-keyed list is not drift")
	}
}

func TestDiffRows_NameKeyedPathUsesTheName(t *testing.T) {
	rows := DiffRows(
		obj(t, `{"containerGroups":[{"name":"default","containers":[{"name":"primary","port":9090}]}]}`),
		obj(t, `{"containerGroups":[{"name":"default","containers":[{"name":"primary","port":8080}]}]}`),
	)

	assert.Equal(t, []string{
		"containerGroups[default].containers[primary].name",
		"containerGroups[default].containers[primary].port",
		"containerGroups[default].name",
	}, rowPaths(rows))

	changed := changedRows(rows)

	require.Len(t, changed, 1)
	assert.Equal(t, "containerGroups[default].containers[primary].port", changed[0].Path)
}

func TestDiffRows_NameKeyedElementMissingLiveIsOneAddition(t *testing.T) {
	rows := DiffRows(
		obj(t, `{"containers":[{"name":"primary","port":8080},{"name":"sidecar","port":9000}]}`),
		obj(t, `{"containers":[{"name":"primary","port":8080}]}`),
	)

	assert.Equal(t, []string{
		"containers[primary].name",
		"containers[primary].port",
		"containers[sidecar]",
	}, rowPaths(rows))

	additions := changedRows(rows)

	require.Len(t, additions, 1, "a whole new container is one addition, not one per field")
	assert.Equal(t, "containers[sidecar]", additions[0].Path)
	assert.True(t, additions[0].Absent)
}

// TestDiffRows_UnkeyedListsCompareWhole covers resourceBundles (plain
// strings) and autoscaling policies (objects with no name). Neither has
// anything to key on, so each list is one leaf and a partial match would
// mean nothing.
func TestDiffRows_UnkeyedListsCompareWhole(t *testing.T) {
	unchanged := DiffRows(
		obj(t, `{"resourceBundles":["gpu.medium"]}`),
		obj(t, `{"resourceBundles":["gpu.medium"]}`),
	)
	require.Len(t, unchanged, 1)
	assert.False(t, unchanged[0].Changed)

	changed := DiffRows(
		obj(t, `{"resourceBundles":["gpu.large"]}`),
		obj(t, `{"resourceBundles":["gpu.medium"]}`),
	)
	require.Len(t, changed, 1)
	assert.True(t, changed[0].Changed)

	policies := DiffRows(
		obj(t, `{"policies":[{"scalingMetric":"gpuCacheUtilization","target":80}]}`),
		obj(t, `{"policies":[{"scalingMetric":"gpuCacheUtilization","target":70}]}`),
	)
	require.Len(t, policies, 1, "an object with no name to key on is one leaf too")
	assert.True(t, policies[0].Changed)
}

func TestDiffRows_EmptyListsAreCompatible(t *testing.T) {
	bothEmpty := DiffRows(obj(t, `{"containers":[]}`), obj(t, `{"containers":[]}`))
	require.Len(t, bothEmpty, 1)
	assert.False(t, bothEmpty[0].Changed)

	liveHasMore := DiffRows(obj(t, `{"containers":[]}`), obj(t, `{"containers":[{"name":"primary"}]}`))
	require.Len(t, liveHasMore, 1, "an empty list has nothing to key on, so it is compared whole")
	assert.True(t, liveHasMore[0].Changed)
}

func TestDiffRows_TypeMismatchIsOneChange(t *testing.T) {
	rows := DiffRows(
		obj(t, `{"probe":{"path":"/health"}}`),
		obj(t, `{"probe":"/health"}`),
	)

	require.Len(t, rows, 1, "an object asked of a scalar is one row, not one per field")
	assert.Equal(t, "probe", rows[0].Path)
	assert.True(t, rows[0].Changed)
	assert.False(t, rows[0].Absent)

	listVsScalar := DiffRows(
		obj(t, `{"entrypoint":["uvicorn","app:app"]}`),
		obj(t, `{"entrypoint":"uvicorn app:app"}`),
	)
	require.Len(t, listVsScalar, 1)
	assert.True(t, listVsScalar[0].Changed)
}

// TestDiffRows_ExplicitNullLiveIsAChangeNotAnAddition separates "the key is
// there holding null" from "the key is not there", which the walk tracks
// with its present flag. Both render differently in a diff.
func TestDiffRows_ExplicitNullLiveIsAChangeNotAnAddition(t *testing.T) {
	rows := DiffRows(
		obj(t, `{"entrypoint":["uvicorn"]}`),
		obj(t, `{"entrypoint":null}`),
	)

	require.Len(t, rows, 1)
	assert.False(t, rows[0].Absent, "the key is there holding null, which is not the same as missing")
	assert.Nil(t, rows[0].Have)
	assert.True(t, rows[0].Changed)
}

func TestDiffRows_EmptyWantAsksForNothing(t *testing.T) {
	assert.Empty(t, DiffRows(obj(t, `{}`), obj(t, `{"anything":1}`)))
	assert.Empty(t, DiffRows(obj(t, `{"probe":{}}`), obj(t, `{"probe":{"path":"/health"}}`)),
		"an object the file leaves empty has no leaves to walk")
}

// TestDiffRows_MemoryIsTheSameSizeWrittenTwoWays is the diff-mode half of
// the memory tolerance: "512MB" in the file and 512000000 in the live
// workload are one sizing written twice, so the leaf reads as context and
// not as a change no deploy would settle. Reporting it as a change would
// mean every run showed permanent drift in a diff that never shrinks.
func TestDiffRows_MemoryIsTheSameSizeWrittenTwoWays(t *testing.T) {
	fileString := DiffRows(
		obj(t, `{"resourceAllocation":{"memory":"512MB"}}`),
		obj(t, `{"resourceAllocation":{"memory":512000000.0}}`),
	)
	require.Len(t, fileString, 1)
	assert.False(t, fileString[0].Changed, "512MB and 512000000 are the same sizing")

	fileNumber := DiffRows(
		obj(t, `{"resourceAllocation":{"memory":512000000.0}}`),
		obj(t, `{"resourceAllocation":{"memory":"512MB"}}`),
	)
	require.Len(t, fileNumber, 1)
	assert.False(t, fileNumber[0].Changed, "the tolerance does not care which side spells it")
}

func TestDiffRows_ADifferentMemoryIsStillDrift(t *testing.T) {
	// 536870912 is 512MiB, a binary size: the platform reads 512MB as
	// 512000000 bytes, so the two are genuinely different sizings.
	rows := DiffRows(
		obj(t, `{"resourceAllocation":{"memory":"512MB"}}`),
		obj(t, `{"resourceAllocation":{"memory":536870912.0}}`),
	)
	require.Len(t, rows, 1)
	assert.True(t, rows[0].Changed, "512MB is not 536870912 bytes")

	spelled := DiffRows(
		obj(t, `{"resourceAllocation":{"memory":"512MB"}}`),
		obj(t, `{"resourceAllocation":{"memory":"1GB"}}`),
	)
	require.Len(t, spelled, 1)
	assert.True(t, spelled[0].Changed, "512MB is not 1GB")
}

// TestDiffRows_NilHaveMeansEveryLeafIsAnAddition is the first deploy: there
// is no live object to agree with, so every leaf of want is an addition,
// walked out to its leaves so a diff can show what will be created rather
// than summarise it.
func TestDiffRows_NilHaveMeansEveryLeafIsAnAddition(t *testing.T) {
	rows := DiffRows(
		obj(t, `{"type":"service","port":8080,"probe":{"path":"/health"},
			"containers":[{"name":"primary","port":8080}],
			"resourceBundles":["gpu.medium"]}`),
		nil,
	)

	assert.Equal(t, []string{
		"containers[primary].name",
		"containers[primary].port",
		"port",
		"probe.path",
		"resourceBundles",
		"type",
	}, rowPaths(rows))

	for _, r := range rows {
		assert.True(t, r.Absent, "leaf %s has nothing live to agree with", r.Path)
		assert.True(t, r.Changed, "leaf %s is an addition on a first deploy", r.Path)
		assert.Nil(t, r.Have)
	}
}

// TestDiffRows_OrderIsStable keeps a diff from reading differently on two
// runs over the same inputs, which would make a diffed CI log useless. The
// inputs are parsed fresh each round: Go randomises map iteration order, and
// the rows must not care.
func TestDiffRows_OrderIsStable(t *testing.T) {
	const (
		wantJSON = `{"zebra":1,"alpha":{"m":2,"a":3},
		"containerGroups":[{"name":"default","containers":[{"name":"primary","port":9090}]}]}`

		haveJSON = `{"zebra":0,"alpha":{"m":0,"a":0},
		"containerGroups":[{"name":"default","containers":[{"name":"primary","port":8080}]}]}`
	)

	first := DiffRows(obj(t, wantJSON), obj(t, haveJSON))
	require.NotEmpty(t, first)

	for range 25 {
		assert.Equal(t, first, DiffRows(obj(t, wantJSON), obj(t, haveJSON)),
			"the same inputs must read the same way twice")
	}
}
