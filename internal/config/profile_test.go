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
	valid := []string{"eu", "eu-mtsaas", "eu_mtsaas", "prod2"}
	for _, name := range valid {
		require.NoError(t, ValidateProfileName(name), "expected %q to be valid", name)
	}

	invalid := []string{"", "eu.mtsaas", "eu mtsaas", "-eu", "eu/prod", "default", "Default", "DEFAULT"}
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

// TestApplyProfile_DefaultNameReserved guards against a "profiles.default"
// section that would be indistinguishable from, yet different than, the
// true top-level default profile in `dr auth profile list`/`show`.
func TestApplyProfile_DefaultNameReserved(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	err := applyProfile("default")
	require.Error(t, err)

	var unknown *UnknownProfileError

	assert.NotErrorAs(t, err, &unknown, "the reserved name should fail validation, not the not-found lookup")
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
