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
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClonePipeline_DefaultName(t *testing.T) {
	installSkipAuth(t)

	var (
		gotMethod string
		gotPath   string
		gotBody   []byte
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "p-2",
			"name": "Clone of wf",
			"mode": "draft",
			"taskNames": ["e1"],
			"createdAt": "2026-04-29T10:00:00Z"
		}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	got, err := ClonePipeline("p-1", "")
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/api/v2/pipelines/p-1/clone", gotPath)

	// No name override → the body must omit "name" entirely.
	var payload map[string]any

	require.NoError(t, json.Unmarshal(gotBody, &payload))

	_, hasName := payload["name"]
	assert.False(t, hasName, "empty name must be omitted from the request body")
	assert.Equal(t, "p-2", got.PipelineID)
	assert.Equal(t, "Clone of wf", got.Name)
	assert.Equal(t, "draft", got.Mode)
	// A clone is always a fresh draft, so the server returns no version/status
	// (mirrors POST /pipelines). The fixture omits them deliberately.
	assert.Zero(t, got.Version)
	assert.Empty(t, got.Status)
}

func TestClonePipeline_NameOverride(t *testing.T) {
	installSkipAuth(t)

	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"p-3","name":"My Clone","mode":"draft","createdAt":"2026-04-29T10:00:00Z"}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	got, err := ClonePipeline("p-1", "My Clone")
	require.NoError(t, err)

	var payload map[string]any

	require.NoError(t, json.Unmarshal(gotBody, &payload))
	assert.Equal(t, "My Clone", payload["name"])
	assert.Equal(t, "My Clone", got.Name)
}

func TestClonePipeline_EscapesPipelineID(t *testing.T) {
	installSkipAuth(t)

	var gotRequestURI string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// RequestURI is the raw, undecoded wire target — unlike r.URL.Path it
		// preserves %2F, so it is the only value that proves the id was escaped
		// (a rewritten "p/1" would arrive here as ".../p/1/clone").
		gotRequestURI = r.RequestURI

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"p-2","name":"Clone of wf","mode":"draft","createdAt":"2026-04-29T10:00:00Z"}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	// A reserved "/" in the id must be percent-encoded so it stays a single
	// path segment instead of altering the request path.
	_, err := ClonePipeline("p/1", "")
	require.NoError(t, err)

	assert.Equal(t, "/api/v2/pipelines/p%2F1/clone", gotRequestURI)
}

func TestClonePipeline_404NotFound(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"Pipeline p-1 not found"}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := ClonePipeline("p-1", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 404")
	assert.Contains(t, err.Error(), "not found")
}
