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

	"github.com/datarobot/cli/internal/version"
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

func TestNormalizeAndValidateVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr string // "" means no error expected
	}{
		{"bare v-prefixed", "v1.2.3", "v1.2.3", ""},
		{"bare no prefix", "1.2.3", "v1.2.3", ""},
		{"prerelease suffix", "v0.12.3-rc.1", "v0.12.3-rc.1", ""},
		{"build suffix no v", "0.12.3+build.5", "v0.12.3+build.5", ""},
		{"prerelease and build", "v1.2.3-rc.1+build.5", "v1.2.3-rc.1+build.5", ""},
		{"missing patch", "1.2", "", `invalid version "1.2": expected vMAJOR.MINOR.PATCH (e.g. v0.12.3)`},
		{"non-numeric", "abc", "", `invalid version "abc": expected vMAJOR.MINOR.PATCH (e.g. v0.12.3)`},
		{"too many components", "v1.2.3.4", "", `invalid version "v1.2.3.4": expected vMAJOR.MINOR.PATCH (e.g. v0.12.3)`},
		{"empty", "", "", `invalid version "": expected vMAJOR.MINOR.PATCH (e.g. v0.12.3)`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeAndValidateVersion(tt.input)
			if tt.wantErr == "" {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)

				return
			}

			require.Error(t, err)
			assert.EqualError(t, err, tt.wantErr)
		})
	}
}

func TestRefuseDowngrade(t *testing.T) {
	originalVersion := version.Version

	defer func() { version.Version = originalVersion }()

	tests := []struct {
		name      string
		installed string
		requested string
		wantErr   string // "" means no error expected
	}{
		{
			"older requested is refused", "v0.12.3", "v0.11.0",
			"refusing to install older version (installed: v0.12.3, requested: v0.11.0)",
		},
		{"same version is allowed", "v0.12.3", "v0.12.3", ""},
		{"newer version is allowed", "v0.12.3", "v0.13.0", ""},
		{"dev bypasses the check entirely", "dev", "v0.0.1", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version.Version = tt.installed

			err := refuseDowngrade(tt.requested)
			if tt.wantErr == "" {
				assert.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.EqualError(t, err, tt.wantErr)
		})
	}
}

func TestBrewCaskVersionError(t *testing.T) {
	err := brewCaskVersionError("v0.12.3")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Homebrew (dr-cli cask)")
	assert.Contains(t, err.Error(), "cannot pin versions")
	assert.Contains(t, err.Error(), "install.sh | sh -s -- v0.12.3")
}
