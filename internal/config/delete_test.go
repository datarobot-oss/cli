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

package config

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/datarobot/cli/internal/testutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeDeleteFixture writes a drconfig.yaml shaped like the VAL-DELETE
// fixtures: root credentials, a global key, and profiles with header, lead,
// and inline comments (including an explicit profiles.default). It returns
// the config file path with viper pointed at it.
func writeDeleteFixture(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)
	viper.Reset()
	t.Cleanup(viper.Reset)

	configDir := filepath.Join(tempDir, ".config", "datarobot")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	configFile := filepath.Join(configDir, "drconfig.yaml")

	raw := `# header comment
endpoint: https://app.datarobot.com/api/v2
token: root-token

# global toggle
ssl_verify: true

profiles:
  # the explicit default profile
  default:
    endpoint: https://default.example.com/api/v2
    token: default-profile-token
  # alpha keeps its own comment
  alpha:
    endpoint: https://alpha.example.com/api/v2 # inline alpha comment
    token: alpha-token
  beta:
    endpoint: https://beta.example.com/api/v2
  gamma:
    endpoint: https://gamma.example.com/api/v2
`
	require.NoError(t, os.WriteFile(configFile, []byte(raw), 0o600))
	require.NoError(t, ReadConfigFile(""))

	return configFile
}

func readDeleteFixtureBytes(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(data)
}

// rawKeys returns the keys of a raw YAML mapping, for order-insensitive
// section-set assertions.
func rawKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))

	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

func TestDeleteProfile_RemovesOnlyNamedSection(t *testing.T) {
	configFile := writeDeleteFixture(t)

	require.NoError(t, DeleteProfile("beta"))

	raw := readRawYAML(t, configFile)

	profiles, ok := raw["profiles"].(map[string]any)
	require.True(t, ok, "expected a profiles: block")

	// beta is gone; every surviving section keeps its exact key/values.
	assert.NotContains(t, profiles, "beta")
	assert.Equal(t, map[string]any{
		"default": map[string]any{
			"endpoint": "https://default.example.com/api/v2",
			"token":    "default-profile-token",
		},
		"alpha": map[string]any{
			"endpoint": "https://alpha.example.com/api/v2",
			"token":    "alpha-token",
		},
		"gamma": map[string]any{
			"endpoint": "https://gamma.example.com/api/v2",
		},
	}, profiles)

	// Root credentials and global values are untouched.
	assert.Equal(t, "https://app.datarobot.com/api/v2", raw["endpoint"])
	assert.Equal(t, "root-token", raw["token"])
	assert.Equal(t, true, raw["ssl_verify"])

	// Comments that do not belong to the deleted section survive.
	out := readDeleteFixtureBytes(t, configFile)
	assert.Contains(t, out, "# header comment")
	assert.Contains(t, out, "# global toggle")
	assert.Contains(t, out, "# the explicit default profile")
	assert.Contains(t, out, "# alpha keeps its own comment")
	assert.Contains(t, out, "# inline alpha comment")

	// The safe-persistence contract keeps the file owner-only.
	info, err := os.Stat(configFile)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestDeleteProfile_NormalizesNameAgainstStoredSection(t *testing.T) {
	configFile := writeDeleteFixture(t)

	// Case and surrounding whitespace normalize onto the stored section,
	// matching --profile selection semantics.
	require.NoError(t, DeleteProfile("  BETA "))

	raw := readRawYAML(t, configFile)

	profiles := raw["profiles"].(map[string]any)
	assert.NotContains(t, profiles, "beta")
	assert.Contains(t, profiles, "alpha")
}

func TestDeleteProfile_MissingNameFailsWithoutChanges(t *testing.T) {
	configFile := writeDeleteFixture(t)

	before := readDeleteFixtureBytes(t, configFile)

	err := DeleteProfile("nope")
	require.Error(t, err)

	var unknown *UnknownProfileError
	require.ErrorAs(t, err, &unknown)
	assert.Equal(t, "nope", unknown.Name)
	assert.Equal(t, []string{"alpha", "beta", "default", "gamma"}, unknown.Known)

	assert.Equal(t, before, readDeleteFixtureBytes(t, configFile))
}

func TestDeleteProfile_MissingNameWithoutProfilesBlock(t *testing.T) {
	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)
	viper.Reset()
	t.Cleanup(viper.Reset)

	configDir := filepath.Join(tempDir, ".config", "datarobot")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	configFile := filepath.Join(configDir, "drconfig.yaml")

	raw := "endpoint: https://app.datarobot.com/api/v2\ntoken: root-token\n"
	require.NoError(t, os.WriteFile(configFile, []byte(raw), 0o600))
	require.NoError(t, ReadConfigFile(""))

	before := readDeleteFixtureBytes(t, configFile)

	err := DeleteProfile("alpha")
	require.Error(t, err)

	var unknown *UnknownProfileError
	require.ErrorAs(t, err, &unknown)
	assert.Empty(t, unknown.Known, "no explicit profiles are defined, so no candidates are listed")

	assert.Equal(t, before, readDeleteFixtureBytes(t, configFile))
}

func TestDeleteProfile_RefusesDefault(t *testing.T) {
	// (a) with an explicit profiles.default section...
	configFile := writeDeleteFixture(t)

	before := readDeleteFixtureBytes(t, configFile)

	err := DeleteProfile("default")
	require.Error(t, err)

	var reserved *ReservedProfileError
	require.ErrorAs(t, err, &reserved)

	assert.Equal(t, before, readDeleteFixtureBytes(t, configFile))

	// ...and (b) without one: the reserved name addresses the root-level
	// credentials, which only logout may clear.
	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)
	viper.Reset()
	t.Cleanup(viper.Reset)

	configDir := filepath.Join(tempDir, ".config", "datarobot")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	rootOnly := filepath.Join(configDir, "drconfig.yaml")

	raw := "endpoint: https://app.datarobot.com/api/v2\ntoken: root-token\n"
	require.NoError(t, os.WriteFile(rootOnly, []byte(raw), 0o600))
	require.NoError(t, ReadConfigFile(""))

	before = readDeleteFixtureBytes(t, rootOnly)

	err = DeleteProfile("default")
	require.Error(t, err)

	require.ErrorAs(t, err, &reserved)

	assert.Equal(t, before, readDeleteFixtureBytes(t, rootOnly))
}

func TestDeleteProfile_InvalidNamesFailWithoutChanges(t *testing.T) {
	configFile := writeDeleteFixture(t)

	before := readDeleteFixtureBytes(t, configFile)

	for _, name := range []string{"", "a/b", ".", ".."} {
		err := DeleteProfile(name)
		require.Error(t, err, "name %q must be rejected", name)
	}

	assert.Equal(t, before, readDeleteFixtureBytes(t, configFile))
}

func TestDeleteProfile_InvalidYAMLFailsWithoutChanges(t *testing.T) {
	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)
	viper.Reset()
	t.Cleanup(viper.Reset)

	configDir := filepath.Join(tempDir, ".config", "datarobot")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	configFile := filepath.Join(configDir, "drconfig.yaml")

	raw := "endpoint: [unclosed\nprofiles: {"
	require.NoError(t, os.WriteFile(configFile, []byte(raw), 0o600))

	// viper.SetConfigFile records the path without parsing, so DeleteProfile
	// itself is the boundary that must refuse the unsound file.
	viper.SetConfigFile(configFile)

	before := readDeleteFixtureBytes(t, configFile)

	err := DeleteProfile("alpha")
	require.Error(t, err)
	assert.Contains(t, err.Error(), configFile, "the error must name the config path")
	assert.Contains(t, err.Error(), "failed to parse config file")

	assert.Equal(t, before, readDeleteFixtureBytes(t, configFile))
}

func TestDeleteProfile_ProfilesNotAMappingFailsWithoutChanges(t *testing.T) {
	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)
	viper.Reset()
	t.Cleanup(viper.Reset)

	configDir := filepath.Join(tempDir, ".config", "datarobot")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	configFile := filepath.Join(configDir, "drconfig.yaml")

	raw := "endpoint: https://app.datarobot.com/api/v2\nprofiles: 42\n"
	require.NoError(t, os.WriteFile(configFile, []byte(raw), 0o600))
	viper.SetConfigFile(configFile)

	before := readDeleteFixtureBytes(t, configFile)

	err := DeleteProfile("alpha")
	require.Error(t, err)
	assert.Contains(t, err.Error(), configFile)
	assert.Contains(t, err.Error(), "not a mapping")

	assert.Equal(t, before, readDeleteFixtureBytes(t, configFile))
}

func TestDeleteProfile_ProfilesNullCountsAsUndefined(t *testing.T) {
	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)
	viper.Reset()
	t.Cleanup(viper.Reset)

	configDir := filepath.Join(tempDir, ".config", "datarobot")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	configFile := filepath.Join(configDir, "drconfig.yaml")

	raw := "endpoint: https://app.datarobot.com/api/v2\nprofiles:\n"
	require.NoError(t, os.WriteFile(configFile, []byte(raw), 0o600))
	viper.SetConfigFile(configFile)

	before := readDeleteFixtureBytes(t, configFile)

	// A null profiles block is "no profiles defined", the same as viper's
	// GetStringMap treats it elsewhere, not an unsound file.
	err := DeleteProfile("alpha")
	require.Error(t, err)

	var unknown *UnknownProfileError
	require.ErrorAs(t, err, &unknown)

	assert.Equal(t, before, readDeleteFixtureBytes(t, configFile))
}

func TestDeleteProfile_UnwritableFileIsLeftUntouched(t *testing.T) {
	configFile := writeDeleteFixture(t)

	require.NoError(t, os.Chmod(configFile, 0o400))
	t.Cleanup(func() { _ = os.Chmod(configFile, 0o600) })

	if os.Geteuid() == 0 {
		t.Skip("root can write a read-only file; the permission boundary does not apply")
	}

	before := readDeleteFixtureBytes(t, configFile)

	err := DeleteProfile("alpha")
	require.Error(t, err, "an unwritable config must fail rather than claim success")
	assert.Contains(t, err.Error(), "failed to write config file")

	assert.Equal(t, before, readDeleteFixtureBytes(t, configFile))
}

func TestDeleteProfile_UnreadableConfigFailsWithoutChanges(t *testing.T) {
	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)
	viper.Reset()
	t.Cleanup(viper.Reset)

	// Point the config at a directory: reading it fails with EISDIR, which
	// DeleteProfile must surface rather than treat as an empty config.
	configFile := filepath.Join(tempDir, "drconfig.yaml")
	require.NoError(t, os.Mkdir(configFile, 0o755))
	viper.SetConfigFile(configFile)

	err := DeleteProfile("alpha")
	require.Error(t, err)
	assert.Contains(t, err.Error(), configFile)
	assert.Contains(t, err.Error(), "failed to read config file")
}

// TestDeleteProfile_LeavesParseableConfigAfterLastOrdinarySection deletes
// every ordinary section: the file must still parse afterwards. The explicit
// profiles.default survives, since the reserved name cannot be deleted.
func TestDeleteProfile_LeavesParseableConfigAfterLastOrdinarySection(t *testing.T) {
	configFile := writeDeleteFixture(t)

	require.NoError(t, DeleteProfile("alpha"))
	require.NoError(t, DeleteProfile("beta"))
	require.NoError(t, DeleteProfile("gamma"))

	raw := readRawYAML(t, configFile)

	profiles, ok := raw["profiles"].(map[string]any)
	require.True(t, ok, "profiles should still be a mapping after the last ordinary section is deleted")
	assert.Equal(t, []string{"default"}, rawKeys(profiles))
}
