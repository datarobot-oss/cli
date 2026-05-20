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

func TestCreateInput_Draft(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v2/pipelines/p-1/inputs", r.URL.Path)

		var body InputCreateRequest

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "v", body.Payload["k"])

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_id":"in-1","pipeline_id":"p-1","is_draft":true,"state":"VALID","payload":{"k":"v"}}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	got, err := CreateInput("p-1", ScopeDraft, nil, map[string]any{"k": "v"})
	require.NoError(t, err)
	assert.Equal(t, "in-1", got.InputID)
	assert.Equal(t, InputStateValid, got.State)
}

func TestCreateInput_LockedURLShape(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/pipelines/p-1/versions/2/inputs", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_id":"in-1","pipeline_id":"p-1","version_id":2,"is_draft":false,"state":"VALID","payload":{}}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	v := 2
	got, err := CreateInput("p-1", ScopeLocked, &v, map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, got.VersionID)
	assert.Equal(t, 2, *got.VersionID)
}

func TestListInputs_AddsPaginationQuery(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "10", r.URL.Query().Get("offset"))
		assert.Equal(t, "5", r.URL.Query().Get("limit"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"input_id":"in-1","pipeline_id":"p-1","is_draft":true,"state":"VALID","payload":{}}]`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	items, err := ListInputs("p-1", ScopeDraft, nil, 10, 5)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "in-1", items[0].InputID)
}

func TestListInputs_OmitsZeroPagination(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.URL.RawQuery)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	items, err := ListInputs("p-1", ScopeDraft, nil, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestGetInput_TargetsCorrectURL(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/pipelines/p-1/inputs/in-1", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_id":"in-1","pipeline_id":"p-1","is_draft":true,"state":"VALID","payload":{}}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	got, err := GetInput("p-1", ScopeDraft, nil, "in-1")
	require.NoError(t, err)
	assert.Equal(t, "in-1", got.InputID)
}

func TestUpdateInput_PatchesDraft(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/api/v2/pipelines/p-1/inputs/in-1", r.URL.Path)

		var body InputUpdateRequest

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "new", body.Payload["k"])

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_id":"in-1","pipeline_id":"p-1","is_draft":true,"state":"VALID","payload":{"k":"new"}}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	got, err := UpdateInput("p-1", "in-1", map[string]any{"k": "new"})
	require.NoError(t, err)
	assert.Equal(t, "new", got.Payload["k"])
}

func TestDeleteInput_LockedURL(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v2/pipelines/p-1/versions/3/inputs/in-1", r.URL.Path)

		w.WriteHeader(http.StatusNoContent)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	v := 3
	require.NoError(t, DeleteInput("p-1", ScopeLocked, &v, "in-1"))
}

func TestDeleteInput_PropagatesAPIError(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"detail":"locked input"}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	err := DeleteInput("p-1", ScopeDraft, nil, "in-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 409")
}
