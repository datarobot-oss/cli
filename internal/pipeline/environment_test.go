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

	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateEnvironment_PostsBody(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v2/pipelines/environments", r.URL.Path)

		var body EnvironmentCreateRequest

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "ml-base", body.Name)

		if assert.NotNil(t, body.Description) {
			assert.Equal(t, "for testing", *body.Description)
		}

		assert.Equal(t, []string{"numpy", "pandas==2.0"}, body.Packages)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"environment_id":"env-1",
			"name":"ml-base",
			"description":"for testing",
			"latest_version":1,
			"versions":[{"version":1,"packages":["numpy","pandas==2.0"],"status":"CREATING","created_at":"t","updated_at":"t"}],
			"created_at":"t","updated_at":"t"
		}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	got, err := CreateEnvironment("ml-base", "for testing", []string{"numpy", "pandas==2.0"})
	require.NoError(t, err)
	assert.Equal(t, "env-1", got.EnvironmentID)
	assert.Equal(t, 1, got.LatestVersion)
	require.Len(t, got.Versions, 1)
	assert.Equal(t, EnvironmentStatusCreating, got.Versions[0].Status)
}

func TestCreateEnvironment_OmitsEmptyDescription(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := map[string]any{}

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&raw))
		_, hasDesc := raw["description"]
		assert.False(t, hasDesc, "description should be omitted when empty")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"environment_id":"env-1","name":"x","latest_version":1,"versions":[],"created_at":"t","updated_at":"t"}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := CreateEnvironment("x", "", []string{"numpy"})
	require.NoError(t, err)
}

func TestListEnvironments_AddsPaginationQuery(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v2/pipelines/environments", r.URL.Path)
		assert.Equal(t, "5", r.URL.Query().Get("offset"))
		assert.Equal(t, "20", r.URL.Query().Get("limit"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"environment_id":"env-1","name":"ml-base","latest_version":2,"latest_status":"READY","created_at":"t","updated_at":"t"}
		]`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	items, err := ListEnvironments(5, 20)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "env-1", items[0].EnvironmentID)
	assert.Equal(t, EnvironmentStatusReady, items[0].LatestStatus)
}

func TestListEnvironments_OmitsZeroPagination(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.URL.RawQuery)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	items, err := ListEnvironments(0, 0)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestUpdateEnvironment_PatchesBody(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/api/v2/pipelines/environments/env-1", r.URL.Path)

		var body EnvironmentUpdateRequest

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, []string{"scikit-learn"}, body.Packages)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"environment_id":"env-1","name":"ml-base","latest_version":2,
			"versions":[
				{"version":2,"packages":["scikit-learn"],"status":"CREATING","created_at":"t","updated_at":"t"},
				{"version":1,"packages":["numpy"],"status":"READY","created_at":"t","updated_at":"t"}
			],
			"created_at":"t","updated_at":"t"
		}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	got, err := UpdateEnvironment("env-1", []string{"scikit-learn"})
	require.NoError(t, err)
	assert.Equal(t, 2, got.LatestVersion)
	require.Len(t, got.Versions, 2)
	assert.Equal(t, 2, got.Versions[0].Version)
}

func TestDeleteEnvironment_HitsCorrectURL(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v2/pipelines/environments/env-1", r.URL.Path)

		w.WriteHeader(http.StatusNoContent)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	require.NoError(t, DeleteEnvironment("env-1"))
}

func TestDeleteEnvironmentVersion_HitsCorrectURL(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v2/pipelines/environments/env-1/versions/3", r.URL.Path)

		w.WriteHeader(http.StatusNoContent)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	require.NoError(t, DeleteEnvironmentVersion("env-1", 3))
}

func TestDeleteEnvironment_PropagatesNotFound(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	err := DeleteEnvironment("nope")

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
}
