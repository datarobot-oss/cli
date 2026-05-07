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
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()

	os.Stdout = old

	var buf bytes.Buffer

	_, _ = io.Copy(&buf, r)

	return buf.String()
}

func makeTestArtifact(id, name, status, catalogID, versionID string) Artifact {
	a := Artifact{
		ID:        id,
		Name:      name,
		Status:    status,
		CreatedAt: time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 10, 14, 30, 0, 0, time.UTC),
	}

	if catalogID != "" {
		a.Spec = Spec{
			ContainerGroups: []ContainerGroup{
				{
					Containers: []Container{
						{
							CodeRef: &CodeRef{
								Datarobot: &DatarobotCodeRef{
									CatalogID:        catalogID,
									CatalogVersionID: versionID,
								},
							},
						},
					},
				},
			},
		}
	}

	return a
}

func TestPrintArtifactJSON_WithCodeRef(t *testing.T) {
	artifact := makeTestArtifact("art-abc-123", "my-agent", "DRAFT", "cat-xyz-789", "fedcba09")

	output := captureStdout(t, func() {
		require.NoError(t, printArtifactJSON(artifact))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Equal(t, "art-abc-123", parsed["id"])
	assert.Equal(t, "my-agent", parsed["name"])
	assert.Equal(t, "DRAFT", parsed["status"])
	assert.Equal(t, "cat-xyz-789", parsed["catalogId"])
	assert.Equal(t, "fedcba09", parsed["versionId"])
	assert.Equal(t, "2026-04-01T08:00:00Z", parsed["createdAt"])
	assert.Equal(t, "2026-04-10T14:30:00Z", parsed["updatedAt"])
}

func TestPrintArtifactJSON_WithoutCodeRef(t *testing.T) {
	artifact := makeTestArtifact("art-abc-123", "my-agent", "DRAFT", "", "")

	output := captureStdout(t, func() {
		require.NoError(t, printArtifactJSON(artifact))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Empty(t, parsed["catalogId"])
	assert.Empty(t, parsed["versionId"])
}

func TestPrintArtifactsJSON(t *testing.T) {
	artifacts := []Artifact{
		makeTestArtifact("art-001", "agent-one", "DRAFT", "cat-001", "ver-001"),
		makeTestArtifact("art-002", "agent-two", "LOCKED", "", ""),
	}

	output := captureStdout(t, func() {
		require.NoError(t, printArtifactsJSON(artifacts))
	})

	var parsed []map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Len(t, parsed, 2)
	assert.Equal(t, "art-001", parsed[0]["id"])
	assert.Equal(t, "ver-001", parsed[0]["versionId"])
	assert.Equal(t, "art-002", parsed[1]["id"])
	assert.Empty(t, parsed[1]["versionId"])
}

func TestPrintArtifactsJSON_Empty(t *testing.T) {
	output := captureStdout(t, func() {
		require.NoError(t, printArtifactsJSON([]Artifact{}))
	})

	var parsed []any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Empty(t, parsed)
}

func TestPrintArtifactDetails_WithCodeRef(t *testing.T) {
	artifact := makeTestArtifact("art-abc-123", "my-agent", "DRAFT", "cat-xyz-789", "fedcba09")

	output := captureStdout(t, func() {
		printArtifactDetails(artifact)
	})

	assert.Contains(t, output, "art-abc-123")
	assert.Contains(t, output, "my-agent")
	assert.Contains(t, output, "DRAFT")
	assert.Contains(t, output, "cat-xyz-789")
	assert.Contains(t, output, "fedcba09")
	assert.Contains(t, output, "2026-04-01 08:00 UTC")
	assert.Contains(t, output, "2026-04-10 14:30 UTC")
	assert.Contains(t, output, "ID:")
	assert.Contains(t, output, "Catalog ID:")
	assert.Contains(t, output, "Version ID:")
}

func TestPrintArtifactDetails_WithoutCodeRef(t *testing.T) {
	artifact := makeTestArtifact("art-abc-123", "my-agent", "DRAFT", "", "")

	output := captureStdout(t, func() {
		printArtifactDetails(artifact)
	})

	assert.Contains(t, output, "Catalog ID:  —")
	assert.Contains(t, output, "Version ID:  —")
}

func TestPrintArtifactsTable_WithCodeRef(t *testing.T) {
	artifacts := []Artifact{
		makeTestArtifact("art-abc-123", "my-agent", "DRAFT", "cat-001", "ver-001"),
	}

	output := captureStdout(t, func() {
		printArtifactsTable(artifacts)
	})

	assert.Contains(t, output, "ARTIFACT ID")
	assert.Contains(t, output, "CATALOG ID")
	assert.Contains(t, output, "VERSION ID")
	assert.Contains(t, output, "art-abc-123")
	assert.Contains(t, output, "my-agent")
	assert.Contains(t, output, "DRAFT")
	assert.Contains(t, output, "cat-001")
	assert.Contains(t, output, "ver-001")
	assert.Contains(t, output, "2026-04-10 14:30 UTC")
}

func TestPrintArtifactsTable_WithoutCodeRef(t *testing.T) {
	artifacts := []Artifact{
		makeTestArtifact("art-abc-123", "my-agent", "DRAFT", "", ""),
	}

	output := captureStdout(t, func() {
		printArtifactsTable(artifacts)
	})

	assert.Equal(t, 2, strings.Count(output, "—"))
}

func TestPrintArtifactsTable_Empty(t *testing.T) {
	output := captureStdout(t, func() {
		printArtifactsTable([]Artifact{})
	})

	assert.Equal(t, "No artifacts found.\n", output)
}
