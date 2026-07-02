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

func TestCreateImage_PostsBody(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v2/pipelines/images", r.URL.Path)

		var body ImageCreateRequest

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "ml-base", body.Name)

		if assert.NotNil(t, body.Description) {
			assert.Equal(t, "for testing", *body.Description)
		}

		assert.Equal(t, []string{"numpy", "pandas==2.0"}, body.Pip)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"id":"img-1",
			"name":"ml-base",
			"description":"for testing",
			"latestVersion":1,
			"versions":[{"version":1,"definition":{"name":"ml-base","pip":["numpy","pandas==2.0"],"nvidia":false},"status":"CREATING","createdAt":"2026-04-29T10:00:00Z","updatedAt":"2026-04-29T10:00:00Z"}],
			"createdAt":"2026-04-29T10:00:00Z","updatedAt":"2026-04-29T10:00:00Z"
		}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	got, err := CreateImage("ml-base", "for testing", []string{"numpy", "pandas==2.0"}, nil, "", false)
	require.NoError(t, err)
	assert.Equal(t, "img-1", got.ImageID)
	assert.Equal(t, 1, got.LatestVersion)
	require.Len(t, got.Versions, 1)
	assert.Equal(t, ImageStatusCreating, got.Versions[0].Status)
}

func TestCreateImage_OmitsEmptyDescription(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := map[string]any{}

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&raw))
		_, hasDesc := raw["description"]
		assert.False(t, hasDesc, "description should be omitted when empty")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"img-1","name":"x","latestVersion":1,"versions":[],"createdAt":"2026-04-29T10:00:00Z","updatedAt":"2026-04-29T10:00:00Z"}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := CreateImage("x", "", []string{"numpy"}, nil, "", false)
	require.NoError(t, err)
}

func TestListImages_AddsPaginationQuery(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v2/pipelines/images", r.URL.Path)
		assert.Equal(t, "5", r.URL.Query().Get("offset"))
		assert.Equal(t, "20", r.URL.Query().Get("limit"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"img-1","name":"ml-base","latestVersion":2,"latestStatus":"READY","createdAt":"2026-04-29T10:00:00Z","updatedAt":"2026-04-29T10:00:00Z"}],"totalCount":1,"count":1}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	items, err := ListImages(5, 20)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "img-1", items[0].ImageID)
	assert.Equal(t, ImageStatusReady, items[0].LatestStatus)
}

func TestListImages_OmitsZeroPagination(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.URL.RawQuery)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"totalCount":0,"count":0}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	items, err := ListImages(0, 0)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestUpdateImage_PatchesBody(t *testing.T) {
	installSkipAuth(t)

	// UpdateImage does a GET first to resolve the canonical name, then PATCHes.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/pipelines/images/img-1", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{
				"id":"img-1","name":"ml-base","latestVersion":1,
				"versions":[
					{"version":1,"definition":{"name":"ml-base","pip":["numpy"],"nvidia":false},"status":"READY","createdAt":"2026-04-29T10:00:00Z","updatedAt":"2026-04-29T10:00:00Z"}
				],
				"createdAt":"2026-04-29T10:00:00Z","updatedAt":"2026-04-29T10:00:00Z"
			}`))
		case http.MethodPatch:
			var body ImageUpdateRequest

			assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "ml-base", body.Name)
			assert.Equal(t, []string{"scikit-learn"}, body.Pip)

			_, _ = w.Write([]byte(`{
				"id":"img-1","name":"ml-base","latestVersion":2,
				"versions":[
					{"version":2,"definition":{"name":"ml-base","pip":["scikit-learn"],"nvidia":false},"status":"CREATING","createdAt":"2026-04-29T10:00:00Z","updatedAt":"2026-04-29T10:00:00Z"},
					{"version":1,"definition":{"name":"ml-base","pip":["numpy"],"nvidia":false},"status":"READY","createdAt":"2026-04-29T10:00:00Z","updatedAt":"2026-04-29T10:00:00Z"}
				],
				"createdAt":"2026-04-29T10:00:00Z","updatedAt":"2026-04-29T10:00:00Z"
			}`))
		default:
			t.Errorf("unexpected method %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	got, err := UpdateImage("img-1", []string{"scikit-learn"}, nil, "", false)
	require.NoError(t, err)
	assert.Equal(t, 2, got.LatestVersion)
	require.Len(t, got.Versions, 2)
	assert.Equal(t, 2, got.Versions[0].Version)
	assert.Equal(t, []string{"scikit-learn"}, got.Versions[0].Definition.Pip)
}

func TestDeleteImage_HitsCorrectURL(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v2/pipelines/images/img-1", r.URL.Path)

		w.WriteHeader(http.StatusNoContent)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	require.NoError(t, DeleteImage("img-1"))
}

func TestDeleteImageVersion_HitsCorrectURL(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v2/pipelines/images/img-1/versions/3", r.URL.Path)

		w.WriteHeader(http.StatusNoContent)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	require.NoError(t, DeleteImageVersion("img-1", 3))
}

func TestDeleteImage_PropagatesNotFound(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	err := DeleteImage("nope")

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
}
