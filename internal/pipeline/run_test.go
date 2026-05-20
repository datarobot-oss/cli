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

package pipeline

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRun_DraftURLAndBody(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v2/pipelines/p-1/dispatches", r.URL.Path)

		var body RunCreateRequest

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "in-1", body.InputID)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"dispatch_id":"d-1","pipeline_id":"p-1","input_id":"in-1","triggered_by":"u","status":"PENDING"}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	got, err := CreateRun("p-1", ScopeDraft, nil, "in-1")
	require.NoError(t, err)
	assert.Equal(t, "d-1", got.RunID)
	assert.Equal(t, RunStatusPending, got.Status)
}

func TestCreateRun_LockedURL(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/pipelines/p-1/versions/2/dispatches", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"dispatch_id":"d-1","pipeline_id":"p-1","version_id":2,"input_id":"in-1","triggered_by":"u","status":"PENDING"}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	v := 2
	got, err := CreateRun("p-1", ScopeLocked, &v, "in-1")
	require.NoError(t, err)
	require.NotNil(t, got.VersionID)
	assert.Equal(t, 2, *got.VersionID)
}

func TestListRuns_QueryAndDecode(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/pipelines/p-1/dispatches", r.URL.Path)
		assert.Equal(t, "10", r.URL.Query().Get("offset"))
		assert.Equal(t, "5", r.URL.Query().Get("limit"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"dispatch_id":"d-1","pipeline_id":"p-1","input_id":"in-1","triggered_by":"u","status":"RUNNING"}]`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	items, err := ListRuns("p-1", ScopeDraft, nil, 10, 5)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, RunStatusRunning, items[0].Status)
}

func TestGetRun_TargetsCorrectURL(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/pipelines/p-1/dispatches/d-1", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"dispatch_id":"d-1","pipeline_id":"p-1","input_id":"in-1","triggered_by":"u","status":"COMPLETED"}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	got, err := GetRun("p-1", ScopeDraft, nil, "d-1")
	require.NoError(t, err)
	assert.Equal(t, RunStatusCompleted, got.Status)
}

func TestGetRunStatus_StatusEndpointURL(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/pipelines/p-1/versions/2/dispatches/d-1/status", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"dispatch_id":"d-1","status":"RUNNING","covalent_dispatch_id":"cov-x"}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	v := 2
	got, err := GetRunStatus("p-1", ScopeLocked, &v, "d-1")
	require.NoError(t, err)
	assert.Equal(t, RunStatusRunning, got.Status)
	assert.Equal(t, "cov-x", got.CovalentDispatchID)
}

func TestCancelRun_DeletesDraftURL(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v2/pipelines/p-1/dispatches/d-1", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	require.NoError(t, CancelRun("p-1", ScopeDraft, nil, "d-1"))
}

func TestCancelRun_PropagatesConflict(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"detail":"already terminal"}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	err := CancelRun("p-1", ScopeDraft, nil, "d-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 409")
	assert.Contains(t, err.Error(), "already terminal")
}
