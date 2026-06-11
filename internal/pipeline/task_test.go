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

func TestGetTask_DraftURLShape(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v2/pipelines/p-1/tasks/1", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":1,
			"pipelineId":"p-1",
			"versionId":null,
			"name":"add",
			"parameters":[{"name":"x","annotation":"int"},{"name":"y","annotation":"int"}],
			"inputs":null,
			"source":"def add(x: int, y: int) -> int:\n    return x + y",
			"resourceBundle":null,
			"taskGroupId":null
		}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	got, err := GetTask("p-1", ScopeDraft, nil, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, got.TaskID)
	assert.Equal(t, "p-1", got.PipelineID)
	assert.Nil(t, got.VersionID)
	assert.Equal(t, "add", got.Name)
	require.Len(t, got.Parameters, 2)
	assert.Equal(t, "x", got.Parameters[0].Name)
	require.NotNil(t, got.Parameters[0].Annotation)
	assert.Equal(t, "int", *got.Parameters[0].Annotation)
	assert.Nil(t, got.Inputs)
	assert.Nil(t, got.ResourceBundle)
	assert.Nil(t, got.TaskGroupID)
}

func TestGetTask_LockedURLShape(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v2/pipelines/p-1/versions/2/tasks/1", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":1,
			"pipelineId":"p-1",
			"versionId":2,
			"name":"add",
			"parameters":[{"name":"x","annotation":"int"}],
			"inputs":{"a":100,"b":200},
			"source":"def add(x: int) -> int:\n    return x",
			"resourceBundle":null,
			"taskGroupId":null
		}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	v := 2
	got, err := GetTask("p-1", ScopeLocked, &v, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, got.TaskID)
	require.NotNil(t, got.VersionID)
	assert.Equal(t, 2, *got.VersionID)
	assert.NotNil(t, got.Inputs)
	assert.InEpsilon(t, float64(100), got.Inputs["a"], 1e-9)
}

func TestGetTask_404ReturnsHTTPError(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"task not found"}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := GetTask("p-1", ScopeDraft, nil, 404)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 404")
}

func TestGetTask_ParameterNilAnnotation(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":1,
			"pipelineId":"p-1",
			"name":"fn",
			"parameters":[{"name":"x"}],
			"source":"def fn(x):\n    pass"
		}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	got, err := GetTask("p-1", ScopeDraft, nil, 1)
	require.NoError(t, err)
	require.Len(t, got.Parameters, 1)
	assert.Nil(t, got.Parameters[0].Annotation)
}
