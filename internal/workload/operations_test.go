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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func operationResponseDoc(status, workloadID string) string {
	return fmt.Sprintf(
		`{"status": %q, "workloadId": %q, "trackVia": "/api/v2/workloads/%s"}`,
		status, workloadID, workloadID,
	)
}

func TestStartWorkload_PostsEmptyBodyAndParsesResponse(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		// Action routes have no trailing slash, unlike the resource route.
		assert.Equal(t, "/api/v2/workloads/wl-1/start", r.URL.Path)

		body, _ := io.ReadAll(r.Body)
		assert.JSONEq(t, `{}`, string(body))

		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, operationResponseDoc("started", "wl-1"))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	resp, err := StartWorkload("wl-1")
	require.NoError(t, err)
	assert.Equal(t, "started", resp.Status)
	assert.Equal(t, "wl-1", resp.WorkloadID)
	assert.Equal(t, "/api/v2/workloads/wl-1", resp.TrackVia)
}

func TestStopWorkload_AlreadyStopped200Succeeds(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v2/workloads/wl-1/stop", r.URL.Path)

		// Stopping an already-stopped workload is an idempotent no-op 200,
		// not an error.
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, operationResponseDoc("Proton is already stopped", "wl-1"))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	resp, err := StopWorkload("wl-1")
	require.NoError(t, err)
	assert.Equal(t, "Proton is already stopped", resp.Status)
}

func TestStartWorkload_409SurfacesServerDetail(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"detail": "The proton must be stopped before attempting to start it."}`)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := StartWorkload("wl-1")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusConflict, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "must be stopped before attempting to start")
}

func TestStopWorkload_404PropagatesAsHTTPError(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := StopWorkload("missing")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
}

func TestStartWorkload_EscapesIDInPath(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// '?' must arrive escaped inside the path segment, never as a query.
		assert.Equal(t, "/api/v2/workloads/wl-1%3Fforce=true/start", r.URL.EscapedPath())
		assert.Empty(t, r.URL.RawQuery)

		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, operationResponseDoc("started", "wl-1"))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := StartWorkload("wl-1?force=true")
	require.NoError(t, err)
}
