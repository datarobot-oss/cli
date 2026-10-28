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
	"testing"

	"github.com/datarobot/cli/internal/testutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProfileScopedKeys_SubsetOfPersistableKeys guards the invariant that
// candidateKeys (write.go) and profileDestPath (write.go) rely on: every
// profile-scoped key must also be persistable, or a bare sweep would never
// consider it a write candidate in the first place, silently making it
// impossible to ever persist through the profile-aware path.
func TestProfileScopedKeys_SubsetOfPersistableKeys(t *testing.T) {
	for key := range ProfileScopedKeys {
		_, ok := PersistableKeys[key]
		assert.True(t, ok, "profile-scoped key %q must also be in PersistableKeys", key)
	}
}

func TestNormalizeProfileName(t *testing.T) {
	assert.Equal(t, "eu-mtsaas", NormalizeProfileName("EU-MTSaaS"))
	assert.Equal(t, "eu-mtsaas", NormalizeProfileName("  eu-mtsaas  "))
	assert.Empty(t, NormalizeProfileName(""))
	assert.Empty(t, NormalizeProfileName("   "))
}

func TestValidateProfileName(t *testing.T) {
	// "default" and its case spellings are legal profile names: an explicit
	// profiles.default section may be selected and written. Only deletion
	// reserves the name. (ValidateProfileName, like before, expects an
	// already-normalized name; ActiveProfile normalizes flag/env values.)
	valid := []string{"eu", "eu-mtsaas", "eu_mtsaas", "prod2", "default", "Default", "DEFAULT"}
	for _, name := range valid {
		require.NoError(t, ValidateProfileName(name), "expected %q to be valid", name)
	}

	invalid := []string{"", "eu.mtsaas", "eu mtsaas", "-eu", "eu/prod"}
	for _, name := range invalid {
		assert.Error(t, ValidateProfileName(name), "expected %q to be invalid", name)
	}
}

func TestUnknownProfileError(t *testing.T) {
	withCandidates := &UnknownProfileError{Name: "eu-mtsaa", Known: []string{"eu-mtsaas", "staging"}}
	assert.Contains(t, withCandidates.Error(), `"eu-mtsaa"`)
	assert.Contains(t, withCandidates.Error(), "eu-mtsaas, staging")

	noCandidates := &UnknownProfileError{Name: "eu"}
	assert.Contains(t, noCandidates.Error(), `"eu"`)
	assert.Contains(t, noCandidates.Error(), "no profiles are configured")
}

func TestProfileNames_Sorted(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("profiles", map[string]any{
		"staging":   map[string]any{"endpoint": "https://staging.example.com"},
		"eu-mtsaas": map[string]any{"endpoint": "https://eu.example.com"},
	})

	assert.Equal(t, []string{"eu-mtsaas", "staging"}, ProfileNames())
}

func TestProfileNames_Empty(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	assert.Empty(t, ProfileNames())
}

func TestApplyProfile_UnknownProfile(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("profiles", map[string]any{
		"staging": map[string]any{"endpoint": "https://staging.example.com"},
	})

	err := applyProfile("eu-mtsaas")
	require.Error(t, err)

	var unknown *UnknownProfileError

	require.ErrorAs(t, err, &unknown)
	assert.Equal(t, "eu-mtsaas", unknown.Name)
	assert.Equal(t, []string{"staging"}, unknown.Known)
}

func TestApplyProfile_InvalidName(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	err := applyProfile("has.dots")
	require.Error(t, err)

	var unknown *UnknownProfileError

	assert.NotErrorAs(t, err, &unknown, "an invalid name should fail validation, not the not-found lookup")
}

// TestApplyProfile_ExplicitDefaultSection covers a defined profiles.default
// section: selecting it merges its own credentials and never falls through
// to the root-level profile's values.
func TestApplyProfile_ExplicitDefaultSection(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.SetConfigFile(writeExplicitDefaultConfig(t))
	require.NoError(t, viper.ReadInConfig())

	require.NoError(t, applyProfile("default"))

	assert.Equal(t, "https://explicit-default.example.com/api/v2", viper.GetString(DataRobotURL))
	assert.Equal(t, "explicit-token", viper.GetString(DataRobotAPIKey))
}

// TestApplyProfile_ExplicitDefaultEndpointOnly covers the atomic endpoint/
// token pair for the explicit default profile: it owns its endpoint and an
// empty token, and must not borrow the root-level profile's token.
func TestApplyProfile_ExplicitDefaultEndpointOnly(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)

	configFile := filepath.Join(tempDir, "drconfig.yaml")
	rawYAML := `endpoint: https://root.example.com/api/v2
token: root-token
profiles:
  default:
    endpoint: https://explicit-default.example.com/api/v2
`
	require.NoError(t, os.WriteFile(configFile, []byte(rawYAML), 0o600))

	viper.SetConfigFile(configFile)
	require.NoError(t, viper.ReadInConfig())

	require.NoError(t, applyProfile(NormalizeProfileName(" default ")))

	assert.Equal(t, "https://explicit-default.example.com/api/v2", viper.GetString(DataRobotURL))
	assert.Empty(t, viper.GetString(DataRobotAPIKey), "an endpoint-only explicit default must not inherit the root token")
}

// TestApplyProfile_ExplicitDefaultMissingSection covers selecting default
// when no profiles.default section exists: the selection is an ordinary
// unknown-profile failure, not a silent root fallback.
func TestApplyProfile_ExplicitDefaultMissingSection(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.SetConfigFile(writeNoDefaultConfig(t))
	require.NoError(t, viper.ReadInConfig())

	err := applyProfile("default")
	require.Error(t, err)

	var unknown *UnknownProfileError

	require.ErrorAs(t, err, &unknown, "an undefined explicit default must fail like any unknown profile")
	assert.Equal(t, "default", unknown.Name)
}

// TestApplyProfile_ExplicitDefaultEmptyMapping covers a bare
// `profiles.default: {}` section: it counts as defined (selection succeeds)
// and owns nothing, so the root-level pair stays effective exactly as for
// any other empty named profile.
func TestApplyProfile_ExplicitDefaultEmptyMapping(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)

	configFile := filepath.Join(tempDir, "drconfig.yaml")
	rawYAML := `endpoint: https://root.example.com/api/v2
token: root-token
profiles:
  default: {}
`
	require.NoError(t, os.WriteFile(configFile, []byte(rawYAML), 0o600))

	viper.SetConfigFile(configFile)
	require.NoError(t, viper.ReadInConfig())

	require.NoError(t, applyProfile("default"), "an empty explicit default mapping counts as a defined section")
	assert.Equal(t, "https://root.example.com/api/v2", viper.GetString(DataRobotURL))
	assert.Equal(t, "root-token", viper.GetString(DataRobotAPIKey))
}

// writeExplicitDefaultConfig writes a scratch config holding root-level
// credentials plus an explicit profiles.default section, and returns its path.
func writeExplicitDefaultConfig(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)

	configFile := filepath.Join(tempDir, "drconfig.yaml")
	rawYAML := `endpoint: https://root.example.com/api/v2
token: root-token
profiles:
  default:
    endpoint: https://explicit-default.example.com/api/v2
    token: explicit-token
`
	require.NoError(t, os.WriteFile(configFile, []byte(rawYAML), 0o600))

	return configFile
}

// writeNoDefaultConfig writes a scratch config with root-level credentials
// and no profiles.default section, and returns its path.
func writeNoDefaultConfig(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)

	configFile := filepath.Join(tempDir, "drconfig.yaml")
	rawYAML := `endpoint: https://root.example.com/api/v2
token: root-token
profiles:
  east:
    endpoint: https://east.example.com/api/v2
`
	require.NoError(t, os.WriteFile(configFile, []byte(rawYAML), 0o600))

	return configFile
}

func TestLoadProfiles_EmptySectionStillListed(t *testing.T) {
	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)
	viper.Reset()
	t.Cleanup(viper.Reset)

	require.NoError(t, CreateConfigFileDirIfNotExists())

	configFile := filepath.Join(tempDir, ".config", "datarobot", "drconfig.yaml")
	rawYAML := `endpoint: https://default.example.com/api/v2
token: default-token
profiles:
  eu-mtsaas:
    endpoint: https://eu.example.com/api/v2
  empty-profile: {}
`
	require.NoError(t, os.WriteFile(configFile, []byte(rawYAML), 0o600))
	require.NoError(t, ReadConfigFile(""))

	def, profiles, err := LoadProfiles()
	require.NoError(t, err)

	assert.Equal(t, "https://default.example.com/api/v2", def.Endpoint)

	names := make([]string, len(profiles))
	for i, p := range profiles {
		names[i] = p.Name
	}

	// AllSettings() silently drops an empty nested map, unlike
	// GetStringMap; empty-profile must still surface here.
	assert.Equal(t, []string{"empty-profile", "eu-mtsaas"}, names)
}

// TestLoadProfiles_ExplicitDefaultListed covers a hand-edited or CLI-written
// "profiles.default" section: it is a legal defined profile now, so it must
// be enumerated alongside the root profile and other named profiles instead
// of being silently dropped.
func TestLoadProfiles_ExplicitDefaultListed(t *testing.T) {
	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)
	viper.Reset()
	t.Cleanup(viper.Reset)

	require.NoError(t, CreateConfigFileDirIfNotExists())

	configFile := filepath.Join(tempDir, ".config", "datarobot", "drconfig.yaml")
	rawYAML := `endpoint: https://default.example.com/api/v2
token: default-token
profiles:
  default:
    endpoint: https://explicit.example.com/api/v2
  eu-mtsaas:
    endpoint: https://eu.example.com/api/v2
`
	require.NoError(t, os.WriteFile(configFile, []byte(rawYAML), 0o600))
	require.NoError(t, ReadConfigFile(""))

	def, profiles, err := LoadProfiles()
	require.NoError(t, err)

	// The root profile stays distinct from the explicit default section.
	assert.Equal(t, "https://default.example.com/api/v2", def.Endpoint)
	assert.Empty(t, def.Name)

	names := make([]string, len(profiles))
	for i, p := range profiles {
		names[i] = p.Name
	}

	assert.Equal(t, []string{"default", "eu-mtsaas"}, names)
	assert.Equal(t, []string{"default", "eu-mtsaas"}, ProfileNames())

	for _, p := range profiles {
		if p.Name == "default" {
			assert.Equal(t, "https://explicit.example.com/api/v2", p.Endpoint)
		}
	}
}

// TestLoadProfiles_MissingFileIsNotAnError covers a config file deleted after
// viper recorded it: SetConfigFile bypasses viper's search, so the failure is
// a plain os.ErrNotExist rather than viper.ConfigFileNotFoundError.
func TestLoadProfiles_MissingFileIsNotAnError(t *testing.T) {
	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)
	viper.Reset()
	t.Cleanup(viper.Reset)

	require.NoError(t, CreateConfigFileDirIfNotExists())

	configFile := filepath.Join(tempDir, ".config", "datarobot", "drconfig.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte("endpoint: https://default.example.com/api/v2\n"), 0o600))
	require.NoError(t, ReadConfigFile(""))
	require.NoError(t, os.Remove(configFile))

	def, profiles, err := LoadProfiles()
	require.NoError(t, err)
	assert.Empty(t, def.Endpoint)
	assert.Empty(t, profiles)
}

func TestApplyProfile_EndpointTokenAtomicity(t *testing.T) {
	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)
	viper.Reset()
	t.Cleanup(viper.Reset)

	require.NoError(t, CreateConfigFileDirIfNotExists())

	configFile := filepath.Join(tempDir, ".config", "datarobot", "drconfig.yaml")
	rawYAML := `endpoint: https://default.example.com/api/v2
token: default-token
profiles:
  partial:
    endpoint: https://partial.example.com/api/v2
`
	require.NoError(t, os.WriteFile(configFile, []byte(rawYAML), 0o600))

	// Load the same way ReadConfigFile does: raw file into viper's config
	// layer, so this test exercises the same layer applyProfile merges into
	// (unlike viper.Set, which writes the override layer above it).
	viper.SetConfigFile(configFile)
	require.NoError(t, viper.ReadInConfig())

	require.NoError(t, applyProfile("partial"))

	assert.Equal(t, "https://partial.example.com/api/v2", viper.GetString(DataRobotURL))
	assert.Empty(t, viper.GetString(DataRobotAPIKey), "token must not inherit the default profile's token")
}
