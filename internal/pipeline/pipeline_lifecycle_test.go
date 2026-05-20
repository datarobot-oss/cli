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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeletePipeline_Success(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v2/pipelines/p-1", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	require.NoError(t, DeletePipeline("p-1"))
}

func TestDeletePipeline_404PropagatesAsHTTPError(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	err := DeletePipeline("p-1")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
}

func TestLockPipeline_PromotesAndDecodes(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/api/v2/pipelines/p-1/mode", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"pipeline_id": "p-1",
			"name": "wf",
			"version": 3,
			"status": "READY",
			"mode": "locked",
			"electron_names": ["e1"],
			"created_at": "2026-04-29T10:00:00Z"
		}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	got, err := LockPipeline("p-1")
	require.NoError(t, err)
	assert.Equal(t, "locked", got.Mode)
	assert.Equal(t, 3, got.Version)
	assert.Equal(t, []string{"e1"}, got.TaskNames)
}

func TestLockPipeline_409Conflict(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"detail":"already locked"}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := LockPipeline("p-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 409")
	assert.Contains(t, err.Error(), "already locked")
}
