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

package wapi

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readHistoryLines returns each non-empty JSONL record in the log as a
// decoded map, preserving order.
func readHistoryLines(t *testing.T, path string) []map[string]any {
	t.Helper()

	f, err := os.Open(path)
	require.NoError(t, err)

	defer func() { _ = f.Close() }()

	var out []map[string]any

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		var obj map[string]any

		err := json.Unmarshal([]byte(line), &obj)
		require.NoError(t, err)

		out = append(out, obj)
	}

	require.NoError(t, sc.Err())

	return out
}

func TestAppendHistory_NotInitialized(t *testing.T) {
	tmp := t.TempDir()

	err := AppendHistory(tmp, HistoryEntry{"op": "init"})
	assert.ErrorIs(t, err, ErrNotInitialized)
}

func TestAppendHistory_WritesOneJSONLine(t *testing.T) {
	tmp := t.TempDir()
	initWapiDir(t, tmp)

	err := AppendHistory(tmp, HistoryEntry{
		"ts":  "2026-04-10T09:15:00Z",
		"op":  "init",
		"foo": "bar",
	})
	require.NoError(t, err)

	entries := readHistoryLines(t, filepath.Join(tmp, DirName, HistoryFile))
	require.Len(t, entries, 1)
	assert.Equal(t, "init", entries[0]["op"])
	assert.Equal(t, "bar", entries[0]["foo"])
}

func TestAppendHistory_MultipleAppendsPreserveOrder(t *testing.T) {
	tmp := t.TempDir()
	initWapiDir(t, tmp)

	ops := []string{"init", "sync", "sync_failed", "sync"}
	for _, op := range ops {
		err := AppendHistory(tmp, HistoryEntry{"op": op})
		require.NoError(t, err)
	}

	entries := readHistoryLines(t, filepath.Join(tmp, DirName, HistoryFile))
	require.Len(t, entries, len(ops))

	for i, op := range ops {
		assert.Equal(t, op, entries[i]["op"])
	}
}

func TestAppendHistory_RotatesAtThreshold(t *testing.T) {
	tmp := t.TempDir()
	initWapiDir(t, tmp)

	path := filepath.Join(tmp, DirName, HistoryFile)
	backup := filepath.Join(tmp, DirName, HistoryBackupFile)

	// Seed the log at exactly the rotation threshold. Truncate creates a
	// sparse file on APFS/ext4, so no 1 MB of zeros is actually written.
	err := os.WriteFile(path, nil, 0o644)
	require.NoError(t, err)
	err = os.Truncate(path, HistoryRotateBytes)
	require.NoError(t, err)

	err = AppendHistory(tmp, HistoryEntry{"op": "sync"})
	require.NoError(t, err)

	_, err = os.Stat(backup)
	require.NoError(t, err, "history.log.1 should exist after rotation")

	entries := readHistoryLines(t, path)
	require.Len(t, entries, 1, "fresh history.log should contain exactly one line")
	assert.Equal(t, "sync", entries[0]["op"])
}

func TestAppendHistory_RotationKeepsOneBackup(t *testing.T) {
	tmp := t.TempDir()
	initWapiDir(t, tmp)

	path := filepath.Join(tmp, DirName, HistoryFile)
	backup := filepath.Join(tmp, DirName, HistoryBackupFile)
	secondBackup := filepath.Join(tmp, DirName, "history.log.2")

	// First rotation (sparse via Truncate, see TestAppendHistory_RotatesAtThreshold).
	err := os.WriteFile(path, nil, 0o644)
	require.NoError(t, err)
	err = os.Truncate(path, HistoryRotateBytes)
	require.NoError(t, err)
	err = AppendHistory(tmp, HistoryEntry{"op": "first"})
	require.NoError(t, err)

	// Force a second rotation by re-inflating the fresh log.
	err = os.Truncate(path, HistoryRotateBytes)
	require.NoError(t, err)
	err = AppendHistory(tmp, HistoryEntry{"op": "second"})
	require.NoError(t, err)

	// Backup exists; no secondary rollup file was created.
	_, err = os.Stat(backup)
	require.NoError(t, err)

	_, err = os.Stat(secondBackup)
	assert.True(t, os.IsNotExist(err), "only one backup should be retained")

	entries := readHistoryLines(t, path)
	require.Len(t, entries, 1)
	assert.Equal(t, "second", entries[0]["op"])
}

// TestAppendHistory_RotationRenameFailureSurfaces pre-seeds the backup path
// as a non-empty directory so os.Rename fails; we assert the error is
// surfaced rather than silently swallowed.
func TestAppendHistory_RotationRenameFailureSurfaces(t *testing.T) {
	tmp := t.TempDir()
	initWapiDir(t, tmp)

	path := filepath.Join(tmp, DirName, HistoryFile)
	backup := filepath.Join(tmp, DirName, HistoryBackupFile)

	err := os.WriteFile(path, nil, 0o644)
	require.NoError(t, err)
	err = os.Truncate(path, HistoryRotateBytes)
	require.NoError(t, err)

	// Make the backup target a non-empty directory so Rename fails.
	err = os.Mkdir(backup, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(backup, "blocker"), []byte("x"), 0o644)
	require.NoError(t, err)

	err = AppendHistory(tmp, HistoryEntry{"op": "sync"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rotate")
}
