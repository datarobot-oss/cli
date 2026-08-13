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
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateWorkloadSettings_PatchesTheRuntimeBlock(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/api/v2/workloads/wl-1/settings/", r.URL.EscapedPath())

		var body map[string]any

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))

		runtime, ok := body["runtime"].(map[string]any)
		if assert.True(t, ok, "the runtime block is sent as a unit") {
			assert.InDelta(t, 3, runtime["replicaCount"], 0)
		}

		fmt.Fprint(w, `{"id":"wl-1","name":"my-app","status":"running"}`)
	}))

	updated, err := UpdateWorkloadSettings("wl-1", json.RawMessage(`{"replicaCount":3}`))
	require.NoError(t, err)
	assert.Equal(t, "wl-1", updated.ID)
	assert.Equal(t, WorkloadStatusRunning, updated.Status)
}

// A id that has to stay one path segment is the same hazard here as
// everywhere else the CLI puts one in a URL.
func TestUpdateWorkloadSettings_EscapesTheID(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/workloads/a%2Fb/settings/", r.URL.EscapedPath())
		fmt.Fprint(w, `{"id":"a/b"}`)
	}))

	_, err := UpdateWorkloadSettings("a/b", json.RawMessage(`{"replicaCount":1}`))
	require.NoError(t, err)
}

// An empty PATCH would come back 200 having changed nothing, which reads as
// success. Refusing locally is the only way that stays distinguishable.
func TestUpdateWorkloadSettings_RefusesAnEmptyRuntime(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("nothing to change must not reach the platform")
	}))

	_, err := UpdateWorkloadSettings("wl-1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no runtime settings")
}

func TestUpdateWorkloadSettings_ErrorCarriesTheServersReason(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"detail":"replicaCount and autoscaling are mutually exclusive"}`)
	}))

	_, err := UpdateWorkloadSettings("wl-1", json.RawMessage(`{"replicaCount":3}`))
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusUnprocessableEntity, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "mutually exclusive")
}
