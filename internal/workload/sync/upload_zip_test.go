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
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildZip_PreservesMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not meaningful on Windows")
	}

	dir := t.TempDir()

	scriptPath := filepath.Join(dir, "entrypoint.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte("#!/bin/sh\n"), 0o644))
	require.NoError(t, os.Chmod(scriptPath, 0o755))

	readmePath := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(readmePath, []byte("hi"), 0o644))

	files := []FileAction{
		{Path: "entrypoint.sh", LocalMode: 0o755},
		{Path: "README.md", LocalMode: 0o644},
	}

	zipPath, err := buildZip(dir, files)
	require.NoError(t, err)

	defer func() { _ = os.Remove(zipPath) }()

	r, err := zip.OpenReader(zipPath)
	require.NoError(t, err)

	defer func() { _ = r.Close() }()

	modes := make(map[string]os.FileMode, len(r.File))
	for _, f := range r.File {
		modes[f.Name] = f.Mode().Perm()
	}

	assert.Equal(t, os.FileMode(0o755), modes["entrypoint.sh"])
	assert.Equal(t, os.FileMode(0o644), modes["README.md"])
}
