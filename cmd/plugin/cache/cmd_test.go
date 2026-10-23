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

package cache

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/plugin"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupCacheEnv isolates the XDG state dir in a temp directory and returns
// the cache file path.
func setupCacheEnv(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()

	testutil.SetTestHomeDir(t, tempDir)
	testutil.SetXDGEnv(t, "XDG_STATE_HOME", tempDir)

	path, err := plugin.DiscoveryCachePath()
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))

	return path
}

func TestClearDeletesCacheFile(t *testing.T) {
	path := setupCacheEnv(t)

	require.NoError(t, os.WriteFile(path, []byte(`{"version":1,"entries":{}}`), 0o600))

	cmd := clearCmd()

	// Silence command output during the test.
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	require.NoError(t, cmd.Execute())

	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err), "cache file must be deleted")
}

func TestClearSucceedsWhenCacheMissing(t *testing.T) {
	setupCacheEnv(t)

	cmd := clearCmd()

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	require.NoError(t, cmd.Execute(), "clearing a missing cache must succeed")
}

func TestBuildStatusOutputMissingCache(t *testing.T) {
	path := setupCacheEnv(t)

	cmd := statusCmd()

	output := buildStatusOutput(cmd, path)

	assert.Equal(t, path, output.Path)
	assert.False(t, output.Exists)
	assert.Zero(t, output.Entries)
	assert.NotEmpty(t, output.TTL)
	assert.Equal(t, "default", output.TTLSource)
}

func TestBuildStatusOutputWithEntries(t *testing.T) {
	path := setupCacheEnv(t)

	file := map[string]any{
		"version": 1,
		"entries": map[string]any{
			"/a": map[string]any{
				"manifest":    map[string]any{"name": "ok"},
				"fingerprint": map[string]any{"mtime": "2026-10-01T12:00:00Z", "size": 10},
				"fetched_at":  "2026-10-01T12:00:00Z",
			},
			"/b": map[string]any{
				"probe_error": "exit status 1",
				"fingerprint": map[string]any{"mtime": "2026-10-01T12:00:00Z", "size": 20},
				"fetched_at":  "2026-10-01T13:00:00Z",
			},
		},
	}

	data, err := json.Marshal(file)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))

	cmd := statusCmd()

	output := buildStatusOutput(cmd, path)

	assert.True(t, output.Exists)
	assert.Equal(t, 2, output.Entries)
	assert.Equal(t, 1, output.OkEntries)
	assert.Equal(t, 1, output.FailedEntries)
	assert.Equal(t, "2026-10-01T12:00:00Z", output.OldestFetch)
	assert.Equal(t, "2026-10-01T13:00:00Z", output.NewestFetch)
	assert.NotZero(t, output.SizeBytes)
}

func TestPrintStatusTable(t *testing.T) {
	output := CacheStatusOutput{
		Path:          "/tmp/cache.json",
		Exists:        true,
		SizeBytes:     123,
		Entries:       2,
		OkEntries:     1,
		FailedEntries: 1,
		OldestFetch:   "2026-10-01T12:00:00Z",
		NewestFetch:   "2026-10-01T13:00:00Z",
		TTL:           "24h0m0s",
		TTLSource:     "default",
	}

	// Table rendering must not error; content assertions stay light since
	// lipgloss styles make exact matching brittle.
	require.NoError(t, printStatusTable(output))
}
