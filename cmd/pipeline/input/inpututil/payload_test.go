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

package inpututil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTempJSON(t *testing.T, name, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, name)

	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func TestResolveFilePath(t *testing.T) {
	t.Run("positional only", func(t *testing.T) {
		got, err := resolveFilePath([]string{"a.json"}, "")
		require.NoError(t, err)
		assert.Equal(t, "a.json", got)
	})

	t.Run("flag only", func(t *testing.T) {
		got, err := resolveFilePath(nil, "b.json")
		require.NoError(t, err)
		assert.Equal(t, "b.json", got)
	})

	t.Run("both supplied", func(t *testing.T) {
		_, err := resolveFilePath([]string{"a.json"}, "b.json")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not both")
	})

	t.Run("neither supplied", func(t *testing.T) {
		_, err := resolveFilePath(nil, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required")
	})
}

func TestResolvePayload_ValidJSON(t *testing.T) {
	path := writeTempJSON(t, "payload.json", `{"key": "value", "n": 7}`)

	got, err := ResolvePayload([]string{path}, "")
	require.NoError(t, err)
	assert.Equal(t, "value", got["key"])
	assert.EqualValues(t, 7, got["n"])
}

func TestResolvePayload_FromFlag(t *testing.T) {
	path := writeTempJSON(t, "payload.json", `{"only": true}`)

	got, err := ResolvePayload(nil, path)
	require.NoError(t, err)
	assert.Equal(t, true, got["only"])
}

func TestResolvePayload_RejectsNonObjectJSON(t *testing.T) {
	path := writeTempJSON(t, "payload.json", `[1, 2, 3]`)

	_, err := ResolvePayload([]string{path}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "JSON object")
}

func TestResolvePayload_MalformedJSON(t *testing.T) {
	path := writeTempJSON(t, "payload.json", `{not json`)

	_, err := ResolvePayload([]string{path}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "JSON")
}

func TestResolvePayload_MissingFile(t *testing.T) {
	_, err := ResolvePayload([]string{"/no/such/file.json"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read /no/such/file.json")
}
