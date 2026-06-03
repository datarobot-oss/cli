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

package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "payload-*.json")

	require.NoError(t, err)

	_, err = f.WriteString(content)

	require.NoError(t, err)
	require.NoError(t, f.Close())

	return f.Name()
}

// ── resolvePayloadFilePath ───────────────────────────────────────────────────

func TestResolvePayloadFilePath_Positional(t *testing.T) {
	path, err := resolvePayloadFilePath([]string{"/some/file.json"}, "")

	require.NoError(t, err)
	assert.Equal(t, "/some/file.json", path)
}

func TestResolvePayloadFilePath_FromFile(t *testing.T) {
	path, err := resolvePayloadFilePath(nil, "/some/file.json")

	require.NoError(t, err)
	assert.Equal(t, "/some/file.json", path)
}

func TestResolvePayloadFilePath_BothProvided(t *testing.T) {
	_, err := resolvePayloadFilePath([]string{"/a.json"}, "/b.json")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not both")
}

func TestResolvePayloadFilePath_NeitherProvided(t *testing.T) {
	_, err := resolvePayloadFilePath(nil, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

// ── ResolvePayload ───────────────────────────────────────────────────────────

func TestResolvePayload_PositionalArg(t *testing.T) {
	path := writeTemp(t, `{"key":"value"}`)

	got, err := ResolvePayload([]string{path}, "")

	require.NoError(t, err)
	assert.Equal(t, map[string]any{"key": "value"}, got)
}

func TestResolvePayload_FromFile(t *testing.T) {
	path := writeTemp(t, `{"count":42}`)

	got, err := ResolvePayload(nil, path)

	require.NoError(t, err)
	assert.Equal(t, map[string]any{"count": float64(42)}, got)
}

func TestResolvePayload_BothProvided(t *testing.T) {
	path := writeTemp(t, `{}`)

	_, err := ResolvePayload([]string{path}, path)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not both")
}

func TestResolvePayload_NeitherProvided(t *testing.T) {
	_, err := ResolvePayload(nil, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestResolvePayload_FileNotFound(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.json")

	_, err := ResolvePayload([]string{missing}, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "read")
}

func TestResolvePayload_InvalidJSON(t *testing.T) {
	path := writeTemp(t, `not json`)

	_, err := ResolvePayload([]string{path}, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
	assert.Contains(t, err.Error(), "JSON object")
}

func TestResolvePayload_NonObjectJSON(t *testing.T) {
	path := writeTemp(t, `[1, 2, 3]`)

	_, err := ResolvePayload([]string{path}, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
	assert.Contains(t, err.Error(), "JSON object")
}
