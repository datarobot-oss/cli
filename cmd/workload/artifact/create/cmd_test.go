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

package create

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload"
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

func makeArtifact(catalogID, catalogVersionID string) workload.Artifact {
	a := workload.Artifact{
		ID:        "art-abc-123",
		Name:      "my-agent",
		Status:    "draft",
		CreatedAt: time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 10, 14, 30, 0, 0, time.UTC),
	}

	if catalogID != "" {
		a.Spec = workload.Spec{
			ContainerGroups: []workload.ContainerGroup{
				{
					Containers: []workload.Container{
						{
							CodeRef: &workload.CodeRef{
								Datarobot: &workload.DatarobotCodeRef{
									CatalogID:        catalogID,
									CatalogVersionID: catalogVersionID,
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

func writeTempFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "spec.json")

	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func TestReadSpecFile_NotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")

	_, err := readSpecFile(path)
	require.Error(t, err)
	assert.Equal(t, "file not found: "+path, err.Error())
}

func TestReadSpecFile_InvalidJSON(t *testing.T) {
	path := writeTempFile(t, "not json")

	_, err := readSpecFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON:")
}

func TestReadSpecFile_Valid(t *testing.T) {
	content := `{"name":"x","spec":{"containerGroups":[{"containers":[{}]}]}}`
	path := writeTempFile(t, content)

	got, err := readSpecFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, string(got))
}

func TestPrintHuman_WithCodeRef(t *testing.T) {
	artifact := makeArtifact("cat-xyz-789", "fedcba09")

	output := captureStdout(t, func() {
		printHuman(artifact)
	})

	assert.Contains(t, output, "ID:          art-abc-123")
	assert.Contains(t, output, "Name:        my-agent")
	assert.Contains(t, output, "Status:      draft")
	assert.Contains(t, output, "Catalog ID:  cat-xyz-789")
	assert.Contains(t, output, "Version ID:  fedcba09")
	assert.Contains(t, output, "Created:     2026-04-01 08:00 UTC")
	assert.Contains(t, output, "Updated:     2026-04-10 14:30 UTC")
}

func TestPrintHuman_WithoutCodeRef(t *testing.T) {
	artifact := makeArtifact("", "")

	output := captureStdout(t, func() {
		printHuman(artifact)
	})

	assert.Contains(t, output, "Catalog ID:  \u2014")
	assert.Contains(t, output, "Version ID:  \u2014")
}

func TestPrintJSON(t *testing.T) {
	artifact := makeArtifact("cat-xyz-789", "fedcba09")

	output := captureStdout(t, func() {
		require.NoError(t, printJSON(artifact))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Equal(t, "art-abc-123", parsed["id"])
	assert.Equal(t, "my-agent", parsed["name"])
	assert.Equal(t, "draft", parsed["status"])
	assert.Equal(t, "cat-xyz-789", parsed["catalogId"])
	assert.Equal(t, "fedcba09", parsed["versionId"])
	assert.Equal(t, "2026-04-01T08:00:00Z", parsed["createdAt"])
	assert.Equal(t, "2026-04-10T14:30:00Z", parsed["updatedAt"])
}

func TestCmd_RejectsArgs(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"unexpected-arg"})

	require.Error(t, cmd.Execute())
}

func TestCmd_MissingSpecFile(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--spec-file is required")
}

func TestCmd_InvalidOutputFormat(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"--output", "yaml"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid output format: yaml")
}
