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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListVersions_TargetsCorrectURL(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/pipelines/p-1/versions", r.URL.Path)
		assert.Equal(t, "10", r.URL.Query().Get("offset"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"version":1,"status":"READY","pythonVersion":"3.12","createdAt":"2026-04-29T10:00:00Z"}],"totalCount":1,"count":1}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	items, err := ListVersions("p-1", 10, 0)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, 1, items[0].Version)
	assert.Equal(t, "READY", items[0].Status)
}

func TestGetVersion_TargetsCorrectURL(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/pipelines/p-1/versions/2", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":2,"status":"READY","pythonVersion":"3.12","createdAt":"2026-04-29T10:00:00Z"}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	got, err := GetVersion("p-1", 2)
	require.NoError(t, err)
	assert.Equal(t, 2, got.Version)
}

func TestGetGraph_DraftURL(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/pipelines/p-1/graph", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"pipeline": {"name":"wf","pythonVersion":"3.12"},
			"nodes": [{"id":0,"type":"function","name":"wf"}],
			"edges": []
		}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	got, err := GetGraph("p-1", ScopeDraft, nil)
	require.NoError(t, err)
	assert.Equal(t, "wf", got.Pipeline.Name)
	require.Len(t, got.Nodes, 1)
	assert.Equal(t, "function", got.Nodes[0].Type)
}

func TestGetGraph_LockedURL(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/pipelines/p-1/versions/3/graph", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pipeline":{"name":"wf","pythonVersion":"3.12"},"nodes":[],"edges":[]}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	v := 3
	got, err := GetGraph("p-1", ScopeLocked, &v)
	require.NoError(t, err)
	assert.Empty(t, got.Nodes)
}
