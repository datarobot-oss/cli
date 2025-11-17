// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSaveURLToConfig_PreservesExistingFields(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "config-auth-test-*")
	require.NoError(t, err)

	defer os.RemoveAll(tempDir)

	originalHome := os.Getenv("HOME")

	os.Setenv("HOME", tempDir)

	defer os.Setenv("HOME", originalHome)

	// Create config directory
	configDir := filepath.Join(tempDir, ".config", "datarobot")

	err = os.MkdirAll(configDir, os.ModePerm)
	require.NoError(t, err)

	configFile := filepath.Join(configDir, "drconfig.yaml")

	// Create initial config with multiple fields
	initialConfig := map[string]interface{}{
		"endpoint":      "https://old.datarobot.com/api/v2",
		"token":         "old-token-12345",
		"ssl_verify":    true,
		"custom_field":  "custom_value",
		"another_field": 42,
	}

	initialYaml, err := yaml.Marshal(initialConfig)
	require.NoError(t, err)

	err = os.WriteFile(configFile, initialYaml, 0o644)
	require.NoError(t, err)

	// Call SaveURLToConfig with a new URL
	err = SaveURLToConfig("https://app.datarobot.com")
	require.NoError(t, err)

	// Read the config file and verify
	rawYaml, err := os.ReadFile(configFile)
	require.NoError(t, err)

	var configMap map[string]interface{}

	err = yaml.Unmarshal(rawYaml, &configMap)
	require.NoError(t, err)

	// Verify endpoint was updated
	assert.Equal(t, "https://app.datarobot.com/api/v2", configMap["endpoint"],
		"Endpoint should be updated")

	// Verify other fields are preserved
	assert.Equal(t, "old-token-12345", configMap["token"],
		"Token should be preserved from original config")
	assert.Equal(t, true, configMap["ssl_verify"],
		"ssl_verify should be preserved")
	assert.Equal(t, "custom_value", configMap["custom_field"],
		"custom_field should be preserved")
	assert.Equal(t, 42, configMap["another_field"],
		"another_field should be preserved")

	// Verify no extra fields were added
	assert.Len(t, configMap, 5, "Config should contain exactly 5 fields")
}

func TestSaveURLToConfig_EmptyURLClearsTokenOnly(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "config-auth-test-*")
	require.NoError(t, err)

	defer os.RemoveAll(tempDir)

	originalHome := os.Getenv("HOME")

	os.Setenv("HOME", tempDir)

	defer os.Setenv("HOME", originalHome)

	configDir := filepath.Join(tempDir, ".config", "datarobot")

	err = os.MkdirAll(configDir, os.ModePerm)
	require.NoError(t, err)

	configFile := filepath.Join(configDir, "drconfig.yaml")

	initialConfig := map[string]interface{}{
		"endpoint":     "https://app.datarobot.com/api/v2",
		"token":        "existing-token",
		"ssl_verify":   false,
		"custom_field": "should_persist",
	}

	initialYaml, err := yaml.Marshal(initialConfig)
	require.NoError(t, err)

	err = os.WriteFile(configFile, initialYaml, 0o644)
	require.NoError(t, err)

	// Call SaveURLToConfig with empty URL
	err = SaveURLToConfig("")
	require.NoError(t, err)

	// Read and verify
	rawYaml, err := os.ReadFile(configFile)
	require.NoError(t, err)

	var configMap map[string]interface{}

	err = yaml.Unmarshal(rawYaml, &configMap)
	require.NoError(t, err)

	// Verify endpoint and token are cleared
	assert.Empty(t, configMap["endpoint"], "Endpoint should be empty")
	assert.Empty(t, configMap["token"], "Token should be empty")

	// Verify other fields are preserved
	assert.Equal(t, false, configMap["ssl_verify"], "ssl_verify should be preserved")
	assert.Equal(t, "should_persist", configMap["custom_field"], "custom_field should be preserved")
}

func TestSaveURLToConfig_CreatesNewFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "config-auth-test-*")
	require.NoError(t, err)

	defer os.RemoveAll(tempDir)

	originalHome := os.Getenv("HOME")

	os.Setenv("HOME", tempDir)

	defer os.Setenv("HOME", originalHome)

	// Don't create the config file - let SaveURLToConfig create it
	err = SaveURLToConfig("https://app.datarobot.com")
	require.NoError(t, err)

	configFile := filepath.Join(tempDir, ".config", "datarobot", "drconfig.yaml")

	// Verify file was created
	assert.FileExists(t, configFile, "Config file should be created")

	// Read and verify content
	rawYaml, err := os.ReadFile(configFile)
	require.NoError(t, err)

	var configMap map[string]interface{}

	err = yaml.Unmarshal(rawYaml, &configMap)
	require.NoError(t, err)

	assert.Equal(t, "https://app.datarobot.com/api/v2", configMap["endpoint"])
}

func TestSaveURLToConfig_WithShortcut(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "config-auth-test-*")
	require.NoError(t, err)

	defer os.RemoveAll(tempDir)

	originalHome := os.Getenv("HOME")

	os.Setenv("HOME", tempDir)

	defer os.Setenv("HOME", originalHome)

	err = SaveURLToConfig("1") // Should expand to US cloud
	require.NoError(t, err)

	configFile := filepath.Join(tempDir, ".config", "datarobot", "drconfig.yaml")

	rawYaml, err := os.ReadFile(configFile)
	require.NoError(t, err)

	var configMap map[string]interface{}

	err = yaml.Unmarshal(rawYaml, &configMap)
	require.NoError(t, err)

	assert.Equal(t, "https://app.datarobot.com/api/v2", configMap["endpoint"])
}

func TestSaveURLToConfig_DoesNotAffectGlobalViper(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "config-auth-test-*")
	require.NoError(t, err)

	defer os.RemoveAll(tempDir)

	originalHome := os.Getenv("HOME")

	os.Setenv("HOME", tempDir)

	defer os.Setenv("HOME", originalHome)

	// Set a value in global viper
	viper.Set("test_key", "test_value")

	originalValue := viper.GetString("test_key")

	// Call SaveURLToConfig
	err = SaveURLToConfig("https://app.datarobot.com")
	require.NoError(t, err)

	// Verify global viper is unchanged
	assert.Equal(t, originalValue, viper.GetString("test_key"),
		"Global viper should not be affected by SaveURLToConfig")

	// Clean up
	viper.Set("test_key", nil)
}
