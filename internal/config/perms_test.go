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

//go:build !windows

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// drconfig.yaml stores the API token in plaintext, so it must never be readable
// by other users on the machine. These tests are POSIX-only: os.Chmod on Windows
// only toggles the read-only bit and mode bits are not meaningful there.

func TestCreateConfigFile_IsOwnerOnly(t *testing.T) {
	testutil.SetTestHomeDir(t, t.TempDir())
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	require.NoError(t, CreateConfigFileDirIfNotExists())

	dir, err := GetConfigDir()
	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(dir, configFileName))
	require.NoError(t, err)

	assert.Equal(t, os.FileMode(configFileMode), info.Mode().Perm(),
		"a freshly created config must be 0600, not os.Create's 0666&^umask")
}

func TestUpdateConfigFile_TightensPreExistingLoosePermissions(t *testing.T) {
	// Configs created by earlier CLI versions are 0644. os.WriteFile cannot fix that
	// on its own - its perm argument is ignored for an existing file - so the write
	// path has to chmod explicitly or the token stays world-readable forever.
	testutil.SetTestHomeDir(t, t.TempDir())
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	dir, err := GetConfigDir()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	configFile := filepath.Join(dir, configFileName)
	require.NoError(t, os.WriteFile(configFile, []byte("endpoint: https://app.datarobot.com/api/v2\n"), 0o644))

	// Sanity check that the fixture really is loose before the write.
	info, err := os.Stat(configFile)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o644), info.Mode().Perm(), "fixture should start world-readable")

	viperx.Set(DataRobotAPIKey, "secret-token")
	viperx.Set(DataRobotURL, "https://app.datarobot.com/api/v2")

	require.NoError(t, ReadConfigFile(configFile))
	require.NoError(t, UpdateConfigFile(DataRobotAPIKey))

	info, err = os.Stat(configFile)
	require.NoError(t, err)

	assert.Equal(t, os.FileMode(configFileMode), info.Mode().Perm(),
		"writing credentials must tighten a pre-existing 0644 config to 0600")

	// The token must actually have been persisted; a permissions-only pass would
	// make this test vacuous.
	contents, err := os.ReadFile(configFile)
	require.NoError(t, err)
	assert.Contains(t, string(contents), "secret-token")
}

func TestConfigFileMode_IsOwnerOnly(t *testing.T) {
	assert.Zero(t, configFileMode&0o077, "no group or other permission bits may be set")
}
