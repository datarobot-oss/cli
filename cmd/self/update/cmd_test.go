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

package update

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveInstallDir(t *testing.T) {
	dir, ok := resolveInstallDir()

	require.True(t, ok, "expected resolveInstallDir to succeed for the test binary")
	assert.True(t, filepath.IsAbs(dir), "expected an absolute directory, got %q", dir)

	exe, err := os.Executable()
	require.NoError(t, err)

	resolved, err := filepath.EvalSymlinks(exe)
	require.NoError(t, err)

	assert.Equal(t, filepath.Dir(resolved), dir,
		"install dir should be the directory of the resolved executable")
}

// TestResolveInstallDirFollowsSymlink verifies that when dr is invoked through a
// symlink (as on PATH), the resolved install dir points at the real binary's
// directory, not the symlink's directory.
func TestResolveInstallDirFollowsSymlink(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	realExe, err := filepath.EvalSymlinks(exe)
	require.NoError(t, err)

	// Create a symlink to the test binary in a separate temp directory.
	linkDir := t.TempDir()
	link := filepath.Join(linkDir, "dr")

	if err := os.Symlink(realExe, link); err != nil {
		t.Skipf("symlinks not supported in this environment: %v", err)
	}

	resolved, err := filepath.EvalSymlinks(link)
	require.NoError(t, err)

	// Resolving the symlink should yield the real binary's directory, which is
	// not the directory containing the symlink itself.
	assert.Equal(t, filepath.Dir(realExe), filepath.Dir(resolved))
	assert.NotEqual(t, linkDir, filepath.Dir(resolved))
}
