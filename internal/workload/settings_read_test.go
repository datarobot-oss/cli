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

package workload

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixture reads a captured platform payload from testdata.
func fixture(t *testing.T, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)

	return string(data)
}

// The fixtures are the platform's own answers, captured on staging: a
// one-group workload on a fixed count, and a group that autoscales.
func TestGetWorkloadSettings_ReadsTheFixture(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/workloads/wl-1/settings/", r.URL.Path)
		fmt.Fprint(w, fixture(t, "workload_settings_stopped.json"))
	}))

	settings, err := GetWorkloadSettings("wl-1")
	require.NoError(t, err)
	assert.Nil(t, settings.Replacement)

	runtime, err := settings.Decode()
	require.NoError(t, err)
	require.Len(t, runtime.ContainerGroups, 1)

	g := runtime.ContainerGroups[0]
	assert.Equal(t, "default", g.Name)
	require.NotNil(t, g.ReplicaCount)
	assert.Equal(t, 1, *g.ReplicaCount)
	assert.False(t, g.Autoscales())
	assert.Equal(t, []string{"cpu.small"}, g.ResourceBundles)
	assert.Equal(t, "availability", g.BundleSelectionPolicy)

	require.Len(t, g.Containers, 1)
	assert.Equal(t, "primary", g.Containers[0].Name)
	assert.InDelta(t, 0.5, g.Containers[0].ResourceAllocation.CPU, 0)
	assert.Equal(t, int64(512_000_000), g.Containers[0].ResourceAllocation.Memory)
	assert.Nil(t, g.Containers[0].ResourceAllocation.GPU)

	require.NotNil(t, g.ResolvedBundle)
	assert.Equal(t, "cpu.small", g.ResolvedBundle.ID)
	assert.Equal(t, int64(536_870_912), g.ResolvedBundle.MemoryBytes)
}

func TestWorkloadSettings_DecodesAnAutoscaledGroup(t *testing.T) {
	var settings WorkloadSettings

	require.NoError(t, json.Unmarshal([]byte(fixture(t, "workload_settings_autoscaled.json")), &settings))

	runtime, err := settings.Decode()
	require.NoError(t, err)

	g := runtime.ContainerGroups[0]
	assert.True(t, g.Autoscales())
	assert.Nil(t, g.ReplicaCount)
	require.NotNil(t, g.Autoscaling.MinReplicaCount)
	assert.Equal(t, 0, *g.Autoscaling.MinReplicaCount)
	assert.Equal(t, 3, *g.Autoscaling.MaxReplicaCount)
	require.Len(t, g.Autoscaling.Policies, 1)
	assert.Equal(t, "httpRequestsConcurrency", g.Autoscaling.Policies[0].ScalingMetric)
	assert.InDelta(t, 2.0, g.Autoscaling.Policies[0].Target, 0)
}

// The route reads a PATCH as the whole runtime, so a replica change re-sends
// everything the platform holds with one number changed, minus the bundle
// the platform resolved, which is its answer and not a request.
func TestWorkloadSettings_WithReplicaCount(t *testing.T) {
	var settings WorkloadSettings

	require.NoError(t, json.Unmarshal([]byte(fixture(t, "workload_settings_stopped.json")), &settings))

	payload, err := settings.WithReplicaCount("default", 3)
	require.NoError(t, err)

	var runtime map[string]any

	require.NoError(t, json.Unmarshal(payload, &runtime))

	groups, _ := runtime["containerGroups"].([]any)
	require.Len(t, groups, 1)

	g, _ := groups[0].(map[string]any)
	assert.InDelta(t, 3, g["replicaCount"], 0)
	assert.NotContains(t, g, "resolvedBundle")
	assert.Equal(t, "availability", g["bundleSelectionPolicy"])
	assert.Equal(t, []any{"cpu.small"}, g["resourceBundles"])

	containers, _ := g["containers"].([]any)
	require.Len(t, containers, 1)

	container, _ := containers[0].(map[string]any)
	allocation, _ := container["resourceAllocation"].(map[string]any)
	assert.InDelta(t, 512_000_000, allocation["memory"], 0, "the allocation travels untouched")

	// The document read is not changed by building a payload from it.
	assert.Contains(t, string(settings.Runtime), "resolvedBundle")
}

func TestWorkloadSettings_WithReplicaCountRefusals(t *testing.T) {
	var fixed, auto WorkloadSettings

	require.NoError(t, json.Unmarshal([]byte(fixture(t, "workload_settings_stopped.json")), &fixed))
	require.NoError(t, json.Unmarshal([]byte(fixture(t, "workload_settings_autoscaled.json")), &auto))

	_, err := fixed.WithReplicaCount("sidecar", 2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `no container group named "sidecar"`)
	assert.Contains(t, err.Error(), "default", "the groups it does have are named")

	_, err = auto.WithReplicaCount("default", 2)
	require.ErrorIs(t, err, ErrGroupAutoscales)
}

// A settings file is either the body the route takes or the runtime block
// a manifest carries; both are read, and neither is sent inside the other.
// What the read prints is a valid file to send back: the envelope is
// unwrapped and the platform's own resolvedBundle is dropped on the way out,
// the same as a replica change drops it.
func TestRuntimeFromSettingsFile_RoundTripsTheJSONOutput(t *testing.T) {
	printed := `{"settings": {"workloadId": "68b0", "runtime": {"containerGroups": [{"name": "default",
		"replicaCount": 1, "resourceBundles": ["cpu.small"], "containers": [{"name": "primary"}],
		"resolvedBundle": {"id": "cpu.small", "cpuCount": 1, "memoryBytes": 536870912}}]}, "replacement": null}}`

	body, err := RuntimeFromSettingsFile(json.RawMessage(printed))
	require.NoError(t, err)
	assert.JSONEq(t, `{"containerGroups": [{"name": "default", "replicaCount": 1,
		"resourceBundles": ["cpu.small"], "containers": [{"name": "primary"}]}]}`, string(body))
}

func TestRuntimeFromSettingsFile(t *testing.T) {
	// A complete group: a name, a container, and a bundle as its resource
	// signal.
	const group = `{"name": "default", "replicaCount": 2, "resourceBundles": ["cpu.small"],
		"containers": [{"name": "primary"}]}`

	body, err := RuntimeFromSettingsFile(json.RawMessage(`{"runtime": {"containerGroups": [` + group + `]}}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"containerGroups": [`+group+`]}`, string(body))

	bare, err := RuntimeFromSettingsFile(json.RawMessage(`{"containerGroups": [` + group + `]}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"containerGroups": [`+group+`]}`, string(bare))

	// An allocation on every container is the other resource signal, and
	// memory may be spelled the manifest's way.
	allocated, err := RuntimeFromSettingsFile(json.RawMessage(`{"containerGroups": [{"name": "default", "replicaCount": 2,
		"containers": [{"name": "primary", "resourceAllocation": {"cpu": 1, "memory": "512MB"}}]}]}`))
	require.NoError(t, err)
	assert.Contains(t, string(allocated), "512MB")

	for name, tc := range map[string]struct{ doc, want string }{
		"neither shape": {`{"name": "my-app", "artifactId": "art-1"}`, "neither a runtime block nor containerGroups"},
		"replicas beside an enabled policy": {
			`{"containerGroups": [{"name": "default", "replicaCount": 2, "resourceBundles": ["cpu.small"],
				"autoscaling": {"enabled": true}, "containers": [{"name": "primary"}]}]}`,
			"replicaCount and autoscaling.enabled=true are mutually exclusive",
		},
		"null runtime":  {`{"runtime": null}`, "neither a runtime block nor containerGroups"},
		"not an object": {`[1, 2]`, "not a JSON object"},
		"nameless group": {
			`{"containerGroups": [{"replicaCount": 2, "containers": [{"name": "primary"}]}]}`,
			"containerGroups[0] needs a name",
		},
		"no containers": {
			`{"containerGroups": [{"name": "default", "replicaCount": 2}]}`,
			"(default) needs at least one container",
		},
		"no resource signal": {
			`{"containerGroups": [{"name": "default", "containers": [{"name": "primary"}]}]}`,
			"(default) needs a resource signal",
		},
		"one container without an allocation": {`{"containerGroups": [{"name": "default", "containers": [
			{"name": "a", "resourceAllocation": {"cpu": 1}}, {"name": "b"}]}]}`, "(default) needs a resource signal"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := RuntimeFromSettingsFile(json.RawMessage(tc.doc))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
