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

func TestParseArtifactStatus(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"draft", "draft"},
		{"DRAFT", "draft"},
		{"Locked", "locked"},
		{"locked", "locked"},
	}

	for _, c := range cases {
		got, err := ParseArtifactStatus(c.in)
		require.NoError(t, err, "input %q", c.in)
		assert.Equal(t, c.want, got, "input %q", c.in)
	}
}

func TestParseArtifactStatus_Invalid(t *testing.T) {
	_, err := ParseArtifactStatus("bogus")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status")
	assert.Contains(t, err.Error(), "bogus")
}

const validMinimalSpec = `{
	"name": "my-agent",
	"spec": {
		"containerGroups": [{
			"containers": [{
				"imageUri": "nginx:latest",
				"port": 8080,
				"resourceRequest": {"cpu": 1, "memory": 536870912}
			}]
		}]
	}
}`

func TestValidateCreateRequest_MinimalValid(t *testing.T) {
	require.NoError(t, ValidateCreateRequest([]byte(validMinimalSpec)))
}

func TestValidateCreateRequest_FullValid(t *testing.T) {
	spec := `{
		"name": "my-agent",
		"description": "demo",
		"spec": {
			"containerGroups": [{
				"containers": [{
					"imageUri": "nginx:latest",
					"port": 8080,
					"resourceRequest": {"cpu": 1, "memory": 536870912},
					"codeRef": {"datarobot": {"catalogId": "c", "catalogVersionId": "v"}}
				}]
			}]
		}
	}`

	require.NoError(t, ValidateCreateRequest([]byte(spec)))
}

func TestValidateCreateRequest_Errors(t *testing.T) {
	cases := []struct {
		name    string
		spec    string
		wantSub string
	}{
		{
			"missing name",
			`{"spec":{"containerGroups":[{"containers":[{}]}]}}`,
			"'name' is missing",
		},
		{
			"empty name",
			`{"name":"","spec":{"containerGroups":[{"containers":[{}]}]}}`,
			"'name' is missing",
		},
		{
			"missing spec",
			`{"name":"x"}`,
			"'spec.containerGroups'",
		},
		{
			"empty container groups",
			`{"name":"x","spec":{"containerGroups":[]}}`,
			"'spec.containerGroups'",
		},
		{
			"empty containers in group",
			`{"name":"x","spec":{"containerGroups":[{"containers":[]}]}}`,
			"[0].containers",
		},
		{
			"unknown top-level field",
			`{"nme":"x","spec":{"containerGroups":[{"containers":[{}]}]}}`,
			`unknown field "nme"`,
		},
		{
			"unknown nested field",
			`{"name":"x","spec":{"containerGroups":[{"containers":[{"imageUrl":"x"}]}]}}`,
			`unknown field "imageUrl"`,
		},
		{
			"wrong type cpu",
			`{"name":"x","spec":{"containerGroups":[{"containers":[{"resourceRequest":{"cpu":"1","memory":1}}]}]}}`,
			"invalid spec",
		},
		{
			"not json",
			`not json`,
			"invalid spec",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateCreateRequest([]byte(c.spec))
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.wantSub)
		})
	}
}
