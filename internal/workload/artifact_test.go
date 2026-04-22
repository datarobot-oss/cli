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
