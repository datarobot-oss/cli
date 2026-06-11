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
	"time"

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

func TestIsSettledWorkloadStatus(t *testing.T) {
	settled := map[string]bool{
		WorkloadStatusUnknown:      true,
		WorkloadStatusSubmitted:    false,
		WorkloadStatusProvisioning: false,
		WorkloadStatusLaunching:    false,
		WorkloadStatusRunning:      true,
		WorkloadStatusSuspended:    true,
		WorkloadStatusInterrupted:  true,
		WorkloadStatusStopping:     false,
		WorkloadStatusStopped:      true,
		WorkloadStatusErrored:      true,
		WorkloadStatusTerminated:   true,
		// Unrecognized future statuses must keep polling rather than be
		// declared settled.
		"future-state": false,
	}

	for status, want := range settled {
		assert.Equalf(t, want, IsSettledWorkloadStatus(status), "status %q", status)
	}
}

func TestWaitForWorkloadStatus_PollsUntilSettled(t *testing.T) {
	installSkipAuth(t)

	calls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++

		status := WorkloadStatusLaunching
		if calls >= 3 {
			status = WorkloadStatusRunning
		}

		fmt.Fprint(w, serverWorkloadDoc("wl-1", "a", status))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	ticks := 0

	wl, err := WaitForWorkloadStatus("wl-1", time.Millisecond, time.Minute, func(w *Workload) {
		ticks++

		require.NotNil(t, w)
	})
	require.NoError(t, err)
	assert.Equal(t, WorkloadStatusRunning, wl.Status)
	assert.Equal(t, 3, calls)
	assert.Equal(t, 3, ticks)
}

func TestWaitForWorkloadStatus_ErroredReturnsWorkloadAndError(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, serverWorkloadDoc("wl-1", "a", WorkloadStatusErrored))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	wl, err := WaitForWorkloadStatus("wl-1", time.Millisecond, time.Minute, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "settled with status errored")
	assert.Contains(t, err.Error(), "dr workload events wl-1")
	require.NotNil(t, wl)
	assert.Equal(t, WorkloadStatusErrored, wl.Status)
}

func TestWaitForWorkloadStatus_AlreadySettledReturnsImmediately(t *testing.T) {
	installSkipAuth(t)

	calls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++

		fmt.Fprint(w, serverWorkloadDoc("wl-1", "a", WorkloadStatusStopped))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	wl, err := WaitForWorkloadStatus("wl-1", time.Millisecond, time.Minute, nil)
	require.NoError(t, err)
	assert.Equal(t, WorkloadStatusStopped, wl.Status)
	assert.Equal(t, 1, calls)
}

func TestWaitForWorkloadStatus_Timeout(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, serverWorkloadDoc("wl-1", "a", WorkloadStatusLaunching))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	wl, err := WaitForWorkloadStatus("wl-1", time.Millisecond, time.Millisecond, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout waiting for workload wl-1")
	// The last observed workload is returned alongside the timeout so
	// callers can still render the in-flight status.
	require.NotNil(t, wl)
	assert.Equal(t, WorkloadStatusLaunching, wl.Status)
}

func TestWaitForWorkloadStatus_GetErrorAborts(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	wl, err := WaitForWorkloadStatus("wl-1", time.Millisecond, time.Minute, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "poll workload wl-1")
	assert.Nil(t, wl)
}
