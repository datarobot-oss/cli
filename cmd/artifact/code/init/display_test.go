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

package initcmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/workload/wapi"
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

func TestPrintLinkedExistingCode_IncludesShortVersion(t *testing.T) {
	out := captureStdout(t, func() {
		printLinkedExistingCode("my-agent", "art-abc-123", "fedcba09")
	})

	assert.Contains(t, out, "Linked to my-agent (art-abc-123) at version fedcba09.")
	assert.Contains(t, out, "Run 'dr artifact code sync' to reconcile any local changes.")
}

func TestPrintLinkedEmptyArtifact_IncludesArtifactName(t *testing.T) {
	out := captureStdout(t, func() {
		printLinkedEmptyArtifact("blank-artifact", "art-empty-001")
	})

	assert.Contains(t, out, "Linked to empty artifact blank-artifact (art-empty-001).")
	assert.Contains(t, out, "Run 'dr artifact code sync' to upload your files.")
}

func TestPrintAlreadyLinked_IncludesPath(t *testing.T) {
	out := captureStdout(t, func() {
		printAlreadyLinked("art-abc-123", "/tmp/proj")
	})

	assert.Contains(t, out, "Already linked to artifact art-abc-123; state exists at "+wapi.Dir("/tmp/proj")+".")
	assert.Contains(t, out, "--force", "the refusal has to name the command that changes the link")
	assert.NotContains(t, out, "Delete "+wapi.Dir("/tmp/proj"),
		"recovery must never end at deleting the state directory")
}

// Re-pointing names both ends, because the id being left is the only way back
// for someone who forced the wrong directory.
func TestPrintRelinked_NamesBothArtifacts(t *testing.T) {
	out := captureStdout(t, func() {
		printRelinked("art-old-111", "art-new-222", "/tmp/proj")
	})

	assert.Contains(t, out, "art-old-111")
	assert.Contains(t, out, "art-new-222")
	assert.NotContains(t, out, "Delete ", "the whole point of --force is not deleting anything")
}

// A link with no readable predecessor still says what it now points at.
func TestPrintRelinked_WithoutAPreviousArtifact(t *testing.T) {
	out := captureStdout(t, func() {
		printRelinked("", "art-new-222", "/tmp/proj")
	})

	assert.Contains(t, out, "art-new-222")
	assert.NotContains(t, out, "from artifact")
}

func TestRenderInitResult_TextWithCodeRef(t *testing.T) {
	catalogID := "cat-xyz-789"
	versionID := "fedcba0987654321"

	out := captureStdout(t, func() {
		require.NoError(t, renderInitResult(outputformat.OutputFormatText, initResult{
			ArtifactID:       "art-abc-123",
			Name:             "my-agent",
			Status:           "draft",
			CatalogID:        &catalogID,
			CatalogVersionID: &versionID,
			Dir:              "/tmp/proj",
		}))
	})

	assert.Contains(t, out, "Linked to my-agent (art-abc-123) at version fedcba09.")
}

func TestRenderInitResult_TextWithoutCodeRef(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, renderInitResult(outputformat.OutputFormatText, initResult{
			ArtifactID: "art-empty-001",
			Name:       "blank-artifact",
			Status:     "draft",
			Dir:        "/tmp/proj",
		}))
	})

	assert.Contains(t, out, "Linked to empty artifact blank-artifact (art-empty-001).")
}

func TestRenderInitResult_JSON_NullsForEmpty(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, renderInitResult(outputformat.OutputFormatJSON, initResult{
			ArtifactID: "art-empty-001",
			Name:       "blank-artifact",
			Status:     "draft",
			Dir:        "/tmp/proj",
		}))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	assert.Nil(t, parsed["catalogId"])
	assert.Nil(t, parsed["catalogVersionId"])
	assert.Equal(t, "/tmp/proj", parsed["dir"])
}

func TestShortVer(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"abc", "abc"},
		{"12345678", "12345678"},
		{"123456789", "12345678"},
		{"fedcba0987654321", "fedcba09"},
	}

	for _, tc := range cases {
		assert.Equal(t, tc.want, shortVer(tc.in), "input=%q", tc.in)
	}
}
