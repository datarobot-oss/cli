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

package sync

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/datarobot/cli/internal/workload/fileops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashEntries_ReportsExecutables(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not meaningful on Windows")
	}

	dir := t.TempDir()

	scriptPath := filepath.Join(dir, "entrypoint.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte("#!/bin/sh\n"), 0o755))

	readmePath := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(readmePath, []byte("hi"), 0o644))

	entries := []fileops.Entry{
		{AbsPath: scriptPath, RelPath: "entrypoint.sh"},
		{AbsPath: readmePath, RelPath: "README.md"},
	}

	manifest, executables, err := hashEntries(entries)
	require.NoError(t, err)

	assert.Len(t, manifest, 2)
	assert.Equal(t, []string{"entrypoint.sh"}, executables)
}

func TestHashEntries_NoExecutablesReportsNone(t *testing.T) {
	dir := t.TempDir()

	readmePath := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(readmePath, []byte("hi"), 0o644))

	entries := []fileops.Entry{{AbsPath: readmePath, RelPath: "README.md"}}

	_, executables, err := hashEntries(entries)
	require.NoError(t, err)
	assert.Empty(t, executables)
}

func TestExecutableWarningMessage(t *testing.T) {
	t.Run("empty when nothing found", func(t *testing.T) {
		assert.Empty(t, executableWarningMessage(nil))
	})

	t.Run("names the count and an example path", func(t *testing.T) {
		msg := executableWarningMessage([]string{"entrypoint.sh", "bin/tool"})
		assert.Contains(t, msg, "2 file(s)")
		assert.Contains(t, msg, "entrypoint.sh")
	})
}
