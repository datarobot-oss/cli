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

func TestPrintAlreadyLinkedHealthy_NoDeleteAdvice(t *testing.T) {
	var buf bytes.Buffer

	printAlreadyLinkedHealthy(&buf, "art-abc-123", "/tmp/proj")

	out := buf.String()

	assert.Contains(t, out, "Already linked to artifact art-abc-123")
	assert.Contains(t, out, wapi.Dir("/tmp/proj"))
	assert.Contains(t, out, "dr artifact code doctor")
	assert.NotContains(t, out, "Delete")
	assert.NotContains(t, out, "rm -rf")
	assert.NotContains(t, out, "re-init")
}

func TestPrintCorruptConfig_NoDeleteAdvice(t *testing.T) {
	var buf bytes.Buffer

	printCorruptConfig(&buf, "/tmp/proj")

	out := buf.String()

	assert.Contains(t, out, "unreadable")
	assert.Contains(t, out, wapi.ConfigPath("/tmp/proj"))
	assert.Contains(t, out, "dr artifact code doctor --fix")
	assert.NotContains(t, out, "Delete")
	assert.NotContains(t, out, "rm -rf")
	assert.NotContains(t, out, "re-init")
}

func TestPrintGoneGuidance_NoDeleteAdvice(t *testing.T) {
	var buf bytes.Buffer

	printGoneGuidance(&buf, "art-gone-001")

	out := buf.String()

	assert.Contains(t, out, "art-gone-001")
	assert.Contains(t, out, "not found")
	assert.Contains(t, out, "dr artifact code doctor --relink <new-artifact-id>")
	assert.NotContains(t, out, "Delete")
	assert.NotContains(t, out, "rm -rf")
	assert.NotContains(t, out, "re-init")
}

func TestPrintMismatchGuidance_NoDeleteAdvice(t *testing.T) {
	var buf bytes.Buffer

	printMismatchGuidance(&buf, "art-mismatch-001")

	out := buf.String()

	assert.Contains(t, out, "art-mismatch-001")
	assert.Contains(t, out, "catalog id no longer matches")
	assert.Contains(t, out, "dr artifact code doctor --relink <new-artifact-id>")
	assert.NotContains(t, out, "Delete")
	assert.NotContains(t, out, "rm -rf")
	assert.NotContains(t, out, "re-init")
}

func TestPrintRelinkSuccess_NoDeleteAdvice(t *testing.T) {
	var buf bytes.Buffer

	printRelinkSuccess(&buf, "art-new-001")

	out := buf.String()

	assert.Contains(t, out, "Relinked to artifact art-new-001")
	assert.Contains(t, out, "sync baseline reset")
	assert.Contains(t, out, "dr artifact code sync")
	assert.NotContains(t, out, "Delete")
	assert.NotContains(t, out, "rm -rf")
}

func TestRenderAlreadyLinkedJSON_PinnedShape(t *testing.T) {
	var buf bytes.Buffer

	artifactID := "art-abc-123"

	renderAlreadyLinkedJSON(&buf, &artifactID, "dr artifact code doctor")

	var parsed map[string]any

	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))

	assert.Equal(t, "error", parsed["status"])
	assert.Equal(t, "already-linked", parsed["error"])
	assert.Equal(t, "art-abc-123", parsed["artifactId"])
	assert.Equal(t, "dr artifact code doctor", parsed["remedy"])
}

func TestRenderAlreadyLinkedJSON_NullArtifactID(t *testing.T) {
	var buf bytes.Buffer

	renderAlreadyLinkedJSON(&buf, nil, "dr artifact code doctor --fix")

	var parsed map[string]any

	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))

	assert.Nil(t, parsed["artifactId"])
	assert.Equal(t, "dr artifact code doctor --fix", parsed["remedy"])
}

func TestRenderRelinkJSON_PureJSON(t *testing.T) {
	var buf bytes.Buffer

	renderRelinkJSON(&buf, "art-new-001", nil)

	var parsed map[string]any

	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))

	assert.Equal(t, "ok", parsed["status"])
	assert.Equal(t, "art-new-001", parsed["artifactId"])
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

// TestNoDeleteAdviceAnywhere is the grep guard: no display function
// produces "Delete", "rm -rf", "remove the state", or "to re-init" advice.
func TestNoDeleteAdviceAnywhere(t *testing.T) {
	dirs := []string{"/tmp/proj", "/tmp/another"}

	for _, dir := range dirs {
		var buf bytes.Buffer

		printAlreadyLinkedHealthy(&buf, "art-1", dir)
		printCorruptConfig(&buf, dir)
		printGoneGuidance(&buf, "art-1")
		printMismatchGuidance(&buf, "art-1")
		printRelinkSuccess(&buf, "art-new")

		out := buf.String()

		for _, bad := range []string{"Delete ", "rm -rf", "remove the state", "to re-init"} {
			assert.NotContains(t, out, bad, "display output must not contain %q: %s", bad, out)
		}
	}
}
