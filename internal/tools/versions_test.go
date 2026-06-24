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

package tools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fullEntry is a well-formed versions.yaml with all required schema fields.
const fullEntry = `echo-tool:
  name: Echo tool
  minimum-version: "1.0.0"
  command: "echo 1.0.0"
  url: https://example.com
  install:
    macos: "echo install"
    linux: "echo install"
`

// writeVersionsYAML writes content to versions.yaml inside dir.
func writeVersionsYAML(t *testing.T, dir, content string) {
	t.Helper()

	err := os.WriteFile(filepath.Join(dir, "versions.yaml"), []byte(content), 0o644)
	require.NoError(t, err)
}

// outsideRepoDir creates a plain temp dir with no .datarobot subtree, chdirs
// into it, and registers cleanup to restore the original working directory.
func outsideRepoDir(t *testing.T) {
	t.Helper()

	dir := t.TempDir()

	origWd, err := os.Getwd()
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, os.Chdir(origWd)) })

	require.NoError(t, os.Chdir(dir))
}

// --- GetRequirementsFromDir ---

func TestGetRequirementsFromDir_ReadsVersionsYaml(t *testing.T) {
	dir := t.TempDir()

	writeVersionsYAML(t, dir, fullEntry)

	prereqs, _, err := GetRequirementsFromDir(dir)

	require.NoError(t, err)
	require.Len(t, prereqs, 1)
	assert.Equal(t, "Echo tool", prereqs[0].Name)
}

func TestGetRequirementsFromDir_ErrorWhenFileAbsent(t *testing.T) {
	prereqs, _, err := GetRequirementsFromDir(t.TempDir())

	require.Error(t, err)
	assert.Nil(t, prereqs)
}

func TestGetRequirementsFromDir_SetsKeyFromMapKey(t *testing.T) {
	const yaml = `my-tool-key:
  name: My Tool
  minimum-version: "1.0.0"
  command: "echo 1.0.0"
  url: https://example.com
  install:
    macos: "echo install"
    linux: "echo install"
`

	dir := t.TempDir()

	writeVersionsYAML(t, dir, yaml)

	prereqs, _, err := GetRequirementsFromDir(dir)

	require.NoError(t, err)
	require.Len(t, prereqs, 1)
	assert.Equal(t, "my-tool-key", prereqs[0].Key)
	assert.Equal(t, "My Tool", prereqs[0].Name)
}

func TestGetRequirementsFromDir_ParsesMultipleEntries(t *testing.T) {
	const yaml = `tool-a:
  name: Tool A
  minimum-version: "1.0.0"
  command: "echo a"
  url: https://example.com/a
  install:
    macos: "echo install"
    linux: "echo install"
tool-b:
  name: Tool B
  minimum-version: "2.0.0"
  command: "echo b"
  url: https://example.com/b
  install:
    macos: "echo install"
    linux: "echo install"
`

	dir := t.TempDir()

	writeVersionsYAML(t, dir, yaml)

	prereqs, _, err := GetRequirementsFromDir(dir)

	require.NoError(t, err)
	assert.Len(t, prereqs, 2)
}

func TestGetRequirementsFromDir_EmptyYamlReturnsNoPrereqs(t *testing.T) {
	dir := t.TempDir()

	writeVersionsYAML(t, dir, "")

	prereqs, _, err := GetRequirementsFromDir(dir)

	require.NoError(t, err)
	assert.Empty(t, prereqs)
}

func TestGetRequirementsFromDir_MalformedYamlReturnsError(t *testing.T) {
	dir := t.TempDir()

	writeVersionsYAML(t, dir, ":\tinvalid: yaml: content [[[")

	_, _, err := GetRequirementsFromDir(dir)

	require.Error(t, err)
}

func TestGetRequirementsFromDir_ReturnsViolationsForMissingFields(t *testing.T) {
	// Entry is missing name, minimum-version, url, and install commands.
	const incompleteYAML = `incomplete-tool:
  command: "echo 1.0.0"
`

	dir := t.TempDir()

	writeVersionsYAML(t, dir, incompleteYAML)

	_, violations, err := GetRequirementsFromDir(dir)

	require.NoError(t, err)
	assert.NotEmpty(t, violations)
}

// --- GetRequirements ---

func TestGetRequirements_ReadsFromRepoRoot(t *testing.T) {
	setupFakeRepoWithVersions(t, fullEntry)

	prereqs, _, err := GetRequirements()

	require.NoError(t, err)
	require.Len(t, prereqs, 1)
	assert.Equal(t, "echo-tool", prereqs[0].Key)
	assert.Equal(t, "Echo tool", prereqs[0].Name)
}

func TestGetRequirements_ErrorWhenOutsideRepo(t *testing.T) {
	outsideRepoDir(t)

	_, _, err := GetRequirements()

	require.Error(t, err)
}

// --- GetSelfRequirement ---

func TestGetSelfRequirement_ReturnsDrEntry(t *testing.T) {
	const drYAML = `dr:
  name: DataRobot CLI
  minimum-version: "1.0.0"
  command: "dr version"
  url: https://example.com
  install:
    macos: "echo install"
    linux: "echo install"
`

	setupFakeRepoWithVersions(t, drYAML)

	self, err := GetSelfRequirement()

	require.NoError(t, err)
	assert.Equal(t, "dr", self.Key)
	assert.Equal(t, "DataRobot CLI", self.Name)
}

func TestGetSelfRequirement_EmptyWhenNoDrEntry(t *testing.T) {
	setupFakeRepoWithVersions(t, fullEntry)

	self, err := GetSelfRequirement()

	require.NoError(t, err)
	assert.Empty(t, self.Key)
	assert.Empty(t, self.Name)
}

func TestGetSelfRequirement_EmptyWhenOutsideRepo(t *testing.T) {
	outsideRepoDir(t)

	self, err := GetSelfRequirement()

	// GetSelfRequirement swallows the repo-not-found error and returns empty.
	require.NoError(t, err)
	assert.Empty(t, self.Key)
}
