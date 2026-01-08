// Copyright 2025 DataRobot, Inc. and its affiliates.
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

package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestWriteConfigFileSilent_OnlyTokenChanged(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "auth-test-*")
	require.NoError(t, err)

	defer os.RemoveAll(tempDir)

	originalHome := os.Getenv("HOME")

	os.Setenv("HOME", tempDir)

	defer os.Setenv("HOME", originalHome)

	viper.Reset()

	defer viper.Reset()

	// Create config directory and file
	err = config.CreateConfigFileDirIfNotExists()
	require.NoError(t, err)

	configDir := filepath.Join(tempDir, ".config", "datarobot")
	configFile := filepath.Join(configDir, "drconfig.yaml")

	// Prepare initial config with endpoint, token, and ssl_verify
	initialConfig := map[string]interface{}{
		"endpoint":   "https://app.datarobot.com/api/v2",
		"token":      "original-token-12345",
		"ssl_verify": true,
	}

	initialYaml, err := yaml.Marshal(initialConfig)
	require.NoError(t, err)

	err = os.WriteFile(configFile, initialYaml, 0o644)
	require.NoError(t, err)

	// Read the config file into viper
	err = config.ReadConfigFile("")
	require.NoError(t, err)

	// Verify initial values are loaded
	assert.Equal(t, "https://app.datarobot.com/api/v2", viper.GetString("endpoint"))
	assert.Equal(t, "original-token-12345", viper.GetString("token"))
	assert.True(t, viper.GetBool("ssl_verify"))

	// Change only the token
	viper.Set("token", "new-token-67890")

	// Call WriteConfigFileSilent
	_ = WriteConfigFileSilent()

	// Reset viper and re-read the file to verify what was actually written
	viper.Reset()

	err = config.ReadConfigFile("")
	require.NoError(t, err)

	// Verify token was changed
	assert.Equal(t, "new-token-67890", viper.GetString("token"), "Token should be updated")

	// Verify endpoint and ssl_verify remain unchanged
	assert.Equal(t, "https://app.datarobot.com/api/v2", viper.GetString("endpoint"), "Endpoint should remain unchanged")
	assert.True(t, viper.GetBool("ssl_verify"), "ssl_verify should remain unchanged")

	// Read the raw YAML to ensure no extra fields were added
	rawYaml, err := os.ReadFile(configFile)
	require.NoError(t, err)

	var configMap map[string]interface{}

	err = yaml.Unmarshal(rawYaml, &configMap)
	require.NoError(t, err)

	// Verify only the expected keys exist
	expectedKeys := []string{"endpoint", "token", "ssl_verify"}

	assert.Len(t, configMap, len(expectedKeys), "Config should contain exactly %d fields", len(expectedKeys))

	for _, key := range expectedKeys {
		assert.Contains(t, configMap, key, "Config should contain key: %s", key)
	}

	// Explicitly verify values one more time from raw YAML
	assert.Equal(t, "https://app.datarobot.com/api/v2", configMap["endpoint"])
	assert.Equal(t, "new-token-67890", configMap["token"])
	assert.Equal(t, true, configMap["ssl_verify"])
}

func TestWriteConfigFileSilent_PreservesExtraFields(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "auth-test-*")
	require.NoError(t, err)

	defer os.RemoveAll(tempDir)

	originalHome := os.Getenv("HOME")

	os.Setenv("HOME", tempDir)

	defer os.Setenv("HOME", originalHome)

	viper.Reset()

	defer viper.Reset()

	err = config.CreateConfigFileDirIfNotExists()
	require.NoError(t, err)

	configDir := filepath.Join(tempDir, ".config", "datarobot")
	configFile := filepath.Join(configDir, "drconfig.yaml")

	// Create config with additional fields that should be preserved
	initialConfig := map[string]interface{}{
		"endpoint":      "https://app.datarobot.com/api/v2",
		"token":         "original-token-12345",
		"ssl_verify":    false,
		"custom_field":  "custom_value",
		"another_field": 42,
	}

	initialYaml, err := yaml.Marshal(initialConfig)
	require.NoError(t, err)

	err = os.WriteFile(configFile, initialYaml, 0o644)
	require.NoError(t, err)

	err = config.ReadConfigFile("")
	require.NoError(t, err)

	// Change only the token
	viper.Set("token", "updated-token-99999")

	_ = WriteConfigFileSilent()

	// Reset and re-read
	viper.Reset()

	err = config.ReadConfigFile("")
	require.NoError(t, err)

	// Verify all original fields are preserved
	assert.Equal(t, "updated-token-99999", viper.GetString("token"))
	assert.Equal(t, "https://app.datarobot.com/api/v2", viper.GetString("endpoint"))
	assert.False(t, viper.GetBool("ssl_verify"))
	assert.Equal(t, "custom_value", viper.GetString("custom_field"))
	assert.Equal(t, 42, viper.GetInt("another_field"))
}

func TestWriteConfigFileSilent_FailsWhenMultipleFieldsChanged(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "auth-test-*")
	require.NoError(t, err)

	defer os.RemoveAll(tempDir)

	originalHome := os.Getenv("HOME")

	os.Setenv("HOME", tempDir)

	defer os.Setenv("HOME", originalHome)

	viper.Reset()

	defer viper.Reset()

	err = config.CreateConfigFileDirIfNotExists()
	require.NoError(t, err)

	configDir := filepath.Join(tempDir, ".config", "datarobot")
	configFile := filepath.Join(configDir, "drconfig.yaml")

	initialConfig := map[string]interface{}{
		"endpoint":   "https://app.datarobot.com/api/v2",
		"token":      "original-token-12345",
		"ssl_verify": true,
	}

	initialYaml, err := yaml.Marshal(initialConfig)
	require.NoError(t, err)

	err = os.WriteFile(configFile, initialYaml, 0o644)
	require.NoError(t, err)

	err = config.ReadConfigFile("")
	require.NoError(t, err)

	// Intentionally modify multiple fields (this demonstrates incorrect usage)
	viper.Set("token", "new-token")
	viper.Set("endpoint", "https://different.datarobot.com/api/v2")
	viper.Set("extra_field", "should_not_exist")

	_ = WriteConfigFileSilent()

	viper.Reset()

	err = config.ReadConfigFile("")
	require.NoError(t, err)

	// Read raw YAML
	rawYaml, err := os.ReadFile(configFile)
	require.NoError(t, err)

	var configMap map[string]interface{}

	err = yaml.Unmarshal(rawYaml, &configMap)
	require.NoError(t, err)

	// This test verifies that WriteConfigFileSilent DOES write all changes
	// (not just the token), which means the caller must be careful to only
	// modify the token field before calling it
	assert.NotEqual(t, initialConfig["endpoint"], configMap["endpoint"],
		"When endpoint is modified in viper, it IS written to the file")
	assert.Contains(t, configMap, "extra_field",
		"When extra fields are added to viper, they ARE written to the file")

	// This test demonstrates that the responsibility for maintaining config
	// integrity lies with the caller, not with WriteConfigFileSilent
}
