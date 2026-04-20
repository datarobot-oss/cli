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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testArtifactJSON = `{
	"id": "art-abc-123",
	"name": "my-agent",
	"status": "DRAFT",
	"spec": {
		"containerGroups": [
			{
				"containers": [
					{
						"codeRef": {
							"datarobot": {
								"catalogId": "cat-xyz-789",
								"catalogVersionId": "fedcba0987654321fedcba09"
							}
						}
					}
				]
			}
		]
	},
	"createdAt": "2026-04-01T08:00:00Z",
	"updatedAt": "2026-04-10T14:30:00Z"
}`

const testArtifactNoCodeRefJSON = `{
	"id": "art-abc-123",
	"name": "my-agent",
	"status": "DRAFT",
	"spec": {
		"containerGroups": [
			{
				"containers": [
					{}
				]
			}
		]
	},
	"createdAt": "2026-04-01T08:00:00Z",
	"updatedAt": "2026-04-10T14:30:00Z"
}`

func TestGetArtifact_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/artifacts/art-abc-123/", r.URL.Path)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.NotEmpty(t, r.Header.Get("User-Agent"))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(testArtifactJSON))
	}))
	defer server.Close()

	artifact, err := GetArtifact(t.Context(), server.URL, "test-token", "art-abc-123")
	require.NoError(t, err)
	assert.Equal(t, "art-abc-123", artifact.ID)
	assert.Equal(t, "my-agent", artifact.Name)
	assert.Equal(t, "DRAFT", artifact.Status)
}

func TestGetArtifact_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	artifact, err := GetArtifact(t.Context(), server.URL, "test-token", "art-abc-123")
	assert.Nil(t, artifact)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "artifact art-abc-123 not found")
}

func TestGetArtifact_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	artifact, err := GetArtifact(t.Context(), server.URL, "test-token", "art-abc-123")
	assert.Nil(t, artifact)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Authentication failed.")
}

func TestGetArtifact_Forbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	artifact, err := GetArtifact(t.Context(), server.URL, "test-token", "art-abc-123")
	assert.Nil(t, artifact)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Authentication failed.")
}

func TestGetArtifact_NetworkError(t *testing.T) {
	artifact, err := GetArtifact(t.Context(), "http://127.0.0.1:1", "test-token", "art-abc-123")
	assert.Nil(t, artifact)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to reach DataRobot:")
}

func TestGetArtifact_UnexpectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	artifact, err := GetArtifact(t.Context(), server.URL, "test-token", "art-abc-123")
	assert.Nil(t, artifact)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected response: 500")
}

func TestGetArtifact_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	artifact, err := GetArtifact(t.Context(), server.URL, "test-token", "art-abc-123")
	assert.Nil(t, artifact)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response:")
}

func TestExtractCodeRef_Found(t *testing.T) {
	var artifact Artifact

	err := json.Unmarshal([]byte(testArtifactJSON), &artifact)
	require.NoError(t, err)

	codeRef := ExtractCodeRef(artifact)
	require.NotNil(t, codeRef)
	assert.Equal(t, "cat-xyz-789", codeRef.CatalogID)
	assert.Equal(t, "fedcba0987654321fedcba09", codeRef.CatalogVersionID)
}

func TestExtractCodeRef_NoContainerGroups(t *testing.T) {
	artifact := Artifact{Spec: Spec{ContainerGroups: nil}}
	assert.Nil(t, ExtractCodeRef(artifact))
}

func TestExtractCodeRef_EmptyContainers(t *testing.T) {
	artifact := Artifact{
		Spec: Spec{
			ContainerGroups: []ContainerGroup{
				{Containers: []Container{}},
			},
		},
	}

	assert.Nil(t, ExtractCodeRef(artifact))
}

func TestExtractCodeRef_NilCodeRef(t *testing.T) {
	artifact := Artifact{
		Spec: Spec{
			ContainerGroups: []ContainerGroup{
				{Containers: []Container{{CodeRef: nil}}},
			},
		},
	}

	assert.Nil(t, ExtractCodeRef(artifact))
}

func TestExtractCodeRef_NilDatarobot(t *testing.T) {
	artifact := Artifact{
		Spec: Spec{
			ContainerGroups: []ContainerGroup{
				{Containers: []Container{{CodeRef: &CodeRef{Datarobot: nil}}}},
			},
		},
	}

	assert.Nil(t, ExtractCodeRef(artifact))
}

func TestExtractCodeRef_NotInResponse(t *testing.T) {
	var artifact Artifact

	err := json.Unmarshal([]byte(testArtifactNoCodeRefJSON), &artifact)
	require.NoError(t, err)

	assert.Nil(t, ExtractCodeRef(artifact))
}
