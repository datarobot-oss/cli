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
