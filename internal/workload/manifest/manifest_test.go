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

package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, validManifest)

	m, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, "my-app", m.Name())
	assert.Equal(t, "68b0aaaa0000000000000001", m.WorkloadID())
	assert.Equal(t, path, m.Path)
	assert.Equal(t, dir, m.Dir)
}

// JSON content parses too: the manifest is whatever the create endpoint
// accepts, and YAML is a superset of JSON.
func TestLoad_JSONContent(t *testing.T) {
	path := writeManifest(t, t.TempDir(), `{"name": "my-app", "artifactId": "68b0bbbb0000000000000002"}`)

	m, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, "my-app", m.Name())
	assert.Empty(t, m.WorkloadID())
}

func TestLoad_EmptyFileRejected(t *testing.T) {
	path := writeManifest(t, t.TempDir(), "")

	_, err := Load(path)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "file is empty")

	// Write reserves the path before it fills it, so a crash in between
	// leaves exactly this file. Naming the fix is what keeps that a
	// self-service recovery rather than a support question.
	assert.Contains(t, err.Error(), "delete it and run the command again")
}

func TestLoad_CommentOnlyFileRejected(t *testing.T) {
	path := writeManifest(t, t.TempDir(), "# nothing here yet\n")

	_, err := Load(path)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "file is empty")
}

func TestLoad_ForeignFileRejected(t *testing.T) {
	path := writeManifest(t, t.TempDir(), `apiVersion: v1
kind: Pod
metadata:
  labels:
    app: my-app
`)

	_, err := Load(path)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "does not look like a DataRobot workload manifest")
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), FileName))
	require.Error(t, err)

	assert.Contains(t, err.Error(), "manifest not found")
}

func TestParse_RootNotMapping(t *testing.T) {
	_, err := Parse([]byte("- one\n- two\n"), "")
	require.Error(t, err)

	assert.Contains(t, err.Error(), "root must be a YAML mapping")
}

func TestParse_InvalidYAML(t *testing.T) {
	_, err := Parse([]byte("name: [unclosed"), "")
	require.Error(t, err)

	assert.Contains(t, err.Error(), "invalid YAML")
}

// A self-referencing anchor parses without complaint and then makes every walk
// in this package run forever. yaml.v3 catches the cycle only when decoding
// into a Go value, which happens after Validate and Compile have walked the
// raw tree, so Parse is the only place that can stop it.
func TestParse_RejectsSelfReferencingAnchor(t *testing.T) {
	_, err := Parse([]byte("name: my-app\nruntime: &rt\n  containerGroups: *rt\n"), "")
	require.Error(t, err)

	assert.Contains(t, err.Error(), `anchor "rt" on line 2 contains itself`)
}

// aliasBomb is ten levels of eight-wide aliasing: a hundred or so nodes on
// disk, over a billion once a walk follows every alias. Nothing is stored
// twice, so neither the file size nor memory use hints at the cost.
const aliasBomb = `name: my-app
artifact:
  l0: &l0 [a, b, c, d, e, f, g, h]
  l1: &l1 [*l0, *l0, *l0, *l0, *l0, *l0, *l0, *l0]
  l2: &l2 [*l1, *l1, *l1, *l1, *l1, *l1, *l1, *l1]
  l3: &l3 [*l2, *l2, *l2, *l2, *l2, *l2, *l2, *l2]
  l4: &l4 [*l3, *l3, *l3, *l3, *l3, *l3, *l3, *l3]
  l5: &l5 [*l4, *l4, *l4, *l4, *l4, *l4, *l4, *l4]
  l6: &l6 [*l5, *l5, *l5, *l5, *l5, *l5, *l5, *l5]
  l7: &l7 [*l6, *l6, *l6, *l6, *l6, *l6, *l6, *l6]
  l8: &l8 [*l7, *l7, *l7, *l7, *l7, *l7, *l7, *l7]
  l9: &l9 [*l8, *l8, *l8, *l8, *l8, *l8, *l8, *l8]
`

func TestParse_RejectsAliasBomb(t *testing.T) {
	_, err := Parse([]byte(aliasBomb), "")
	require.Error(t, err)

	assert.Contains(t, err.Error(), "too large")
}

// Every walk in this package recurses once per level of the document, and the
// node cap does not bound that: one long chain of nesting is one node per
// level, so it passes the count while the walk descends the whole way.
//
// The bound comes from yaml.Unmarshal, which refuses the document before any
// walk sees it. That is a guarantee of the library rather than of this package,
// which is the reason to pin it: were a future version to lift its limit, this
// fails here rather than as a stack overflow inside Validate.
func TestParse_RejectsRunawayNesting(t *testing.T) {
	const depth = 100_000

	// One node per level, so nothing here is near the node cap.
	require.Less(t, depth, maxExpandedNodes)

	_, err := Parse([]byte("name: my-app\nartifact: "+strings.Repeat("[", depth)+strings.Repeat("]", depth)+"\n"), "")
	require.Error(t, err)

	assert.Contains(t, err.Error(), "exceeded max depth")
}

// What the guard rejects is unbounded expansion, not anchors. Reusing one is
// ordinary YAML, and the value still has to arrive intact at every use.
func TestParse_KeepsReusedAnchors(t *testing.T) {
	m, err := Parse([]byte(`name: my-app
x-shared: &shared
  - name: OPENAI_API_KEY
    value: dr-credential:68b0cccc0000000000000003/apiToken
artifact:
  spec:
    containerGroups:
      - name: default
        containers:
          - name: primary
            environmentVars: *shared
          - name: sidecar
            environmentVars: *shared
`), "")
	require.NoError(t, err)

	compiled, err := m.Compile()
	require.NoError(t, err)

	require.Len(t, compiled.CredentialRefs, 2)
	assert.Equal(t, "68b0cccc0000000000000003", compiled.CredentialRefs[0].CredentialID)
	assert.Equal(t, "apiToken", compiled.CredentialRefs[0].Key)
}

func TestManifest_WorkloadIDAbsent(t *testing.T) {
	m, err := Parse([]byte("name: my-app\n"), "")
	require.NoError(t, err)

	assert.Empty(t, m.WorkloadID())
}

func TestLocate_StartDir(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, validManifest)

	found, err := Locate(dir)
	require.NoError(t, err)

	assert.Equal(t, path, found)
}

func TestLocate_AncestorDir(t *testing.T) {
	root := t.TempDir()
	path := writeManifest(t, root, validManifest)

	nested := filepath.Join(root, "services", "api", "handlers")
	require.NoError(t, os.MkdirAll(nested, 0o750))

	found, err := Locate(nested)
	require.NoError(t, err)

	assert.Equal(t, path, found)
}

func TestLocate_NotFound(t *testing.T) {
	_, err := Locate(t.TempDir())
	require.Error(t, err)

	assert.ErrorIs(t, err, ErrNotFound)
}

// A directory named .datarobot.yaml is not a manifest; the walk keeps
// climbing past it.
func TestLocate_IgnoresDirectory(t *testing.T) {
	root := t.TempDir()
	path := writeManifest(t, root, validManifest)

	child := filepath.Join(root, "sub")
	require.NoError(t, os.MkdirAll(filepath.Join(child, FileName), 0o750))

	found, err := Locate(child)
	require.NoError(t, err)

	assert.Equal(t, path, found)
}

// The suffix is printed inside commands the reader is meant to copy, so a
// project directory with a space in it has to survive the paste. Unquoted it
// becomes a --dir that stops at the space with a stray argument behind it,
// pointed at a directory nobody has.
func TestDirFlag_QuotesAPathWithASpace(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	spaced := filepath.Join(cwd, "My Projects", "app")

	assert.Equal(t, ` --dir 'My Projects/app'`, DirFlag(spaced))
}

// Quoting an ordinary path would be noise on every message that carries one.
// The path is built with the OS separator and comes out with forward slashes,
// which is the one spelling every shell hands over as it is.
func TestDirFlag_LeavesAnOrdinaryPathBare(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	assert.Equal(t, " --dir services/api", DirFlag(filepath.Join(cwd, "services", "api")))
}

// The project directory is the one the command was run from, so there is
// nothing to point it at.
func TestDirFlag_SaysNothingAboutTheCurrentDirectory(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	assert.Empty(t, DirFlag(cwd))
}

// A shell does something with far more than a space. An allowlist is what
// keeps the list from having to be guessed at.
func TestDirFlag_QuotesWhatAShellWouldActOn(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	for _, name := range []string{"a&b", "proj$HOME", "back`tick`", "semi;colon", "sub*shell", "quo'te"} {
		flag := DirFlag(filepath.Join(cwd, name))

		assert.True(t, strings.HasPrefix(flag, " --dir '"), "%s was left bare: %s", name, flag)
		assert.True(t, strings.HasSuffix(flag, "'"), "%s was not closed: %s", name, flag)
	}
}

// An ordinary path stays bare, or every message that carries one is littered
// with quotes it does not need.
func TestDirFlag_LeavesAnOrdinaryPathUnquoted(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	assert.Equal(t, " --dir services/api-2_v1.0",
		DirFlag(filepath.Join(cwd, "services", "api-2_v1.0")))
}
