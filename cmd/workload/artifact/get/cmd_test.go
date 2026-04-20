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

package get

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
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

func TestPrintJSON_WithCodeRef(t *testing.T) {
	artifact := workload.Artifact{
		ID:     "art-abc-123",
		Name:   "my-agent",
		Status: "DRAFT",
		Spec: workload.Spec{
			ContainerGroups: []workload.ContainerGroup{
				{
					Containers: []workload.Container{
						{
							CodeRef: &workload.CodeRef{
								Datarobot: &workload.DatarobotCodeRef{
									CatalogID:        "cat-xyz-789",
									CatalogVersionID: "fedcba09",
								},
							},
						},
					},
				},
			},
		},
		CreatedAt: time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 10, 14, 30, 0, 0, time.UTC),
	}

	output := captureStdout(t, func() {
		err := printJSON(artifact)
		require.NoError(t, err)
	})

	var parsed map[string]interface{}

	err := json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "art-abc-123", parsed["id"])
	assert.Equal(t, "my-agent", parsed["name"])
	assert.Equal(t, "DRAFT", parsed["status"])
	assert.Equal(t, "fedcba09", parsed["version"])
	assert.Equal(t, "cat-xyz-789", parsed["catalog"])
	assert.Equal(t, "2026-04-01T08:00:00Z", parsed["createdAt"])
	assert.Equal(t, "2026-04-10T14:30:00Z", parsed["updatedAt"])
}

func TestPrintJSON_WithoutCodeRef(t *testing.T) {
	artifact := workload.Artifact{
		ID:        "art-abc-123",
		Name:      "my-agent",
		Status:    "DRAFT",
		Spec:      workload.Spec{},
		CreatedAt: time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 10, 14, 30, 0, 0, time.UTC),
	}

	output := captureStdout(t, func() {
		err := printJSON(artifact)
		require.NoError(t, err)
	})

	var parsed map[string]interface{}

	err := json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err)
	assert.Empty(t, parsed["version"])
	assert.Empty(t, parsed["catalog"])
}

func TestPrintHuman_WithCodeRef(t *testing.T) {
	artifact := workload.Artifact{
		ID:     "art-abc-123",
		Name:   "my-agent",
		Status: "DRAFT",
		Spec: workload.Spec{
			ContainerGroups: []workload.ContainerGroup{
				{
					Containers: []workload.Container{
						{
							CodeRef: &workload.CodeRef{
								Datarobot: &workload.DatarobotCodeRef{
									CatalogID:        "cat-xyz-789",
									CatalogVersionID: "fedcba09",
								},
							},
						},
					},
				},
			},
		},
		CreatedAt: time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 10, 14, 30, 0, 0, time.UTC),
	}

	output := captureStdout(t, func() {
		printHuman(artifact)
	})

	assert.Contains(t, output, "ID:       art-abc-123")
	assert.Contains(t, output, "Name:     my-agent")
	assert.Contains(t, output, "Status:   DRAFT")
	assert.Contains(t, output, "Version:  fedcba09")
	assert.Contains(t, output, "Catalog:  cat-xyz-789")
	assert.Contains(t, output, "Created:  2026-04-01 08:00 UTC")
	assert.Contains(t, output, "Updated:  2026-04-10 14:30 UTC")
}

func TestPrintHuman_WithoutCodeRef(t *testing.T) {
	artifact := workload.Artifact{
		ID:        "art-abc-123",
		Name:      "my-agent",
		Status:    "DRAFT",
		Spec:      workload.Spec{},
		CreatedAt: time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 10, 14, 30, 0, 0, time.UTC),
	}

	output := captureStdout(t, func() {
		printHuman(artifact)
	})

	assert.Contains(t, output, "Version:  \u2014")
	assert.Contains(t, output, "Catalog:  \u2014")
}

func TestCmd_RequiresArg(t *testing.T) {
	cmd := Cmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.Error(t, err)
}
