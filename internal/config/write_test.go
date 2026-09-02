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
	"gopkg.in/yaml.v3"
)

// setupWriteTest sets up an isolated home dir + viper instance and returns
// the path UpdateConfigFile will write to.
func setupWriteTest(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)
	viper.Reset()
	t.Cleanup(viper.Reset)

	require.NoError(t, CreateConfigFileDirIfNotExists())

	return filepath.Join(tempDir, ".config", "datarobot", "drconfig.yaml")
}

func readRawYAML(t *testing.T, path string) map[string]any {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var m map[string]any

	require.NoError(t, yaml.Unmarshal(data, &m))

	return m
}

func TestUpdateConfigFile_ExplicitWriteLandsUnderActiveProfile(t *testing.T) {
	configFile := setupWriteTest(t)

	viper.Set(ProfileKey, "eu-mtsaas")
	viper.Set(DataRobotURL, "https://app.eu.datarobot.com/api/v2")
	viper.Set(DataRobotAPIKey, "eu-token")

	require.NoError(t, UpdateConfigFile(DataRobotURL, DataRobotAPIKey))

	raw := readRawYAML(t, configFile)

	profiles, ok := raw["profiles"].(map[string]any)
	require.True(t, ok, "expected a profiles: block")

	eu, ok := profiles["eu-mtsaas"].(map[string]any)
	require.True(t, ok, "expected profiles.eu-mtsaas to be created")

	assert.Equal(t, "https://app.eu.datarobot.com/api/v2", eu["endpoint"])
	assert.Equal(t, "eu-token", eu["token"])

	// The default (top-level) endpoint/token must not exist yet: this is a
	// brand-new profile, nothing was ever written at the top level.
	assert.NotContains(t, raw, "endpoint")
	assert.NotContains(t, raw, "token")
}

func TestUpdateConfigFile_TopLevelUnchangedAfterProfileWrite(t *testing.T) {
	configFile := setupWriteTest(t)

	initial := "endpoint: https://app.datarobot.com/api/v2\ntoken: default-token\n"
	require.NoError(t, os.WriteFile(configFile, []byte(initial), 0o600))
	require.NoError(t, ReadConfigFile(""))

	viper.Set(ProfileKey, "eu-mtsaas")
	viper.Set(DataRobotURL, "https://app.eu.datarobot.com/api/v2")
	viper.Set(DataRobotAPIKey, "eu-token")

	require.NoError(t, UpdateConfigFile(DataRobotURL, DataRobotAPIKey))

	raw := readRawYAML(t, configFile)

	assert.Equal(t, "https://app.datarobot.com/api/v2", raw["endpoint"], "default profile's endpoint must be untouched")
	assert.Equal(t, "default-token", raw["token"], "default profile's token must be untouched")
}

func TestUpdateConfigFile_PreservesComments(t *testing.T) {
	configFile := setupWriteTest(t)

	initial := "# my datarobot config\nendpoint: https://app.datarobot.com/api/v2\ntoken: default-token\n"
	require.NoError(t, os.WriteFile(configFile, []byte(initial), 0o600))
	require.NoError(t, ReadConfigFile(""))

	viper.Set(DataRobotAPIKey, "new-token")
	require.NoError(t, UpdateConfigFile(DataRobotAPIKey))

	data, err := os.ReadFile(configFile)
	require.NoError(t, err)

	var doc yaml.Node

	require.NoError(t, yaml.Unmarshal(data, &doc))
	require.Len(t, doc.Content, 1)

	root := doc.Content[0]
	require.NotEmpty(t, root.Content)
	assert.Equal(t, "my datarobot config", root.Content[0].HeadComment[2:], "head comment should be preserved verbatim")
}

func TestUpdateConfigFile_BareSweepDoesNotForkInheritedKeyIntoProfile(t *testing.T) {
	configFile := setupWriteTest(t)

	// eu-mtsaas exists but defines nothing of its own yet: endpoint, token,
	// and ca-cert all come from the default profile via ReadConfigFile's
	// merge.
	initial := "endpoint: https://app.datarobot.com/api/v2\ntoken: default-token\nca-cert: /etc/ssl/corp.pem\nprofiles:\n  eu-mtsaas: {}\n"
	require.NoError(t, os.WriteFile(configFile, []byte(initial), 0o600))

	viper.Set(ProfileKey, "eu-mtsaas")
	require.NoError(t, ReadConfigFile(""))

	// A bare sweep (no explicit keys) must not copy any of those inherited
	// values down into profiles.eu-mtsaas.
	require.NoError(t, UpdateConfigFile())

	raw := readRawYAML(t, configFile)

	profiles, ok := raw["profiles"].(map[string]any)
	require.True(t, ok)

	eu, ok := profiles["eu-mtsaas"].(map[string]any)
	require.True(t, ok)
	assert.Empty(t, eu, "inherited default-profile values must not be forked into the profile by a bare sweep")
}

func TestUpdateConfigFile_GlobalKeyWritesTopLevelUnderActiveProfile(t *testing.T) {
	configFile := setupWriteTest(t)

	initial := "endpoint: https://app.datarobot.com/api/v2\ntoken: default-token\n"
	require.NoError(t, os.WriteFile(configFile, []byte(initial), 0o600))
	require.NoError(t, ReadConfigFile(""))

	viper.Set(ProfileKey, "eu-mtsaas")
	viper.Set(DefaultLLMID, "some-deployment-id")

	require.NoError(t, UpdateConfigFile(DefaultLLMID))

	raw := readRawYAML(t, configFile)

	assert.Equal(t, "some-deployment-id", raw[DefaultLLMID], "default-llm-id is global, so it writes at the top level even with a profile active")
	assert.NotContains(t, raw, "profiles")
}

func TestUpdateConfigFile_ProfileNameWrittenLowercase(t *testing.T) {
	configFile := setupWriteTest(t)

	viper.Set(ProfileKey, "EU-MTSaaS")
	viper.Set(DataRobotURL, "https://app.eu.datarobot.com/api/v2")
	viper.Set(DataRobotAPIKey, "eu-token")

	require.NoError(t, UpdateConfigFile(DataRobotURL, DataRobotAPIKey))

	raw := readRawYAML(t, configFile)

	profiles, ok := raw["profiles"].(map[string]any)
	require.True(t, ok)

	_, hasLowercase := profiles["eu-mtsaas"]
	assert.True(t, hasLowercase, "the profile section must be written lowercase so ReadConfigFile's case-insensitive lookup finds it again")
}
