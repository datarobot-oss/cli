// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestEnvironment(t *testing.T) (*httptest.Server, func()) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "auth-test-*")
	require.NoError(t, err)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)

	// Save original callback function.
	originalCallback := apiKeyCallbackFunc

	viper.Reset()

	// Create mock DataRobot API server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/version/" {
			// Check authorization header.
			auth := r.Header.Get("Authorization")
			if auth == "Bearer valid-token" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"version":"10.0.0"}`))

				return
			}

			if auth == "Bearer expired-token" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"message":"Invalid credentials"}`))

				return
			}

			w.WriteHeader(http.StatusUnauthorized)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	viper.Set(config.DataRobotURL, server.URL+"/api/v2")
	err = config.CreateConfigFileDirIfNotExists()
	require.NoError(t, err)

	cleanup := func() {
		server.Close()
		os.Setenv("HOME", originalHome)
		os.RemoveAll(tempDir)
		viper.Reset()

		apiKeyCallbackFunc = originalCallback
	}

	return server, cleanup
}

func TestEnsureAuthenticated_MissingCredentials(t *testing.T) {
	server, cleanup := setupTestEnvironment(t)
	defer cleanup()

	viper.Set(config.DataRobotAPIKey, "")
	os.Unsetenv("DATAROBOT_API_TOKEN")

	// Mock the callback to simulate failure to retrieve API key.
	apiKeyCallbackFunc = func(_ string) (string, error) {
		return "", errors.New("simulated authentication failure")
	}

	// EnsureAuthenticated should detect missing credentials.
	result := EnsureAuthenticated()
	assert.False(t, result, "Expected EnsureAuthenticated to return false with missing credentials")

	// Verify the URL is properly configured.
	baseURL := config.GetBaseURL()
	assert.Equal(t, server.URL, baseURL, "Expected base URL to be set from test server")
}

func TestEnsureAuthenticated_ExpiredCredentials(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	viper.Set(config.DataRobotAPIKey, "expired-token")
	os.Unsetenv("DATAROBOT_API_TOKEN")

	apiKey := config.GetAPIKey()
	assert.Empty(t, apiKey, "Expected GetAPIKey to return empty string for expired token")

	// Mock the callback to simulate failure to refresh expired credentials.
	apiKeyCallbackFunc = func(_ string) (string, error) {
		return "", errors.New("simulated authentication failure")
	}

	result := EnsureAuthenticated()
	assert.False(t, result, "Expected EnsureAuthenticated to return false with expired credentials")
}

func TestEnsureAuthenticated_ValidCredentials(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	viper.Set(config.DataRobotAPIKey, "valid-token")
	os.Unsetenv("DATAROBOT_API_TOKEN")

	apiKey := config.GetAPIKey()
	assert.Equal(t, "valid-token", apiKey, "Expected GetAPIKey to return valid token")

	result := EnsureAuthenticated()
	assert.True(t, result, "Expected EnsureAuthenticated to return true with valid credentials")
}

func TestEnsureAuthenticated_ValidEnvironmentToken(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	os.Setenv("DATAROBOT_API_TOKEN", "valid-token")
	viper.Set(config.DataRobotAPIKey, "")

	apiKey := config.GetAPIKey()
	assert.Equal(t, "valid-token", apiKey, "Expected GetAPIKey to return valid token from environment")

	result := EnsureAuthenticated()
	assert.True(t, result, "Expected EnsureAuthenticated to return true with valid environment credentials")
}

func TestEnsureAuthenticated_NoURL(t *testing.T) {
	// Create temp directory for test config.
	tempDir, err := os.MkdirTemp("", "auth-test-*")
	require.NoError(t, err)

	defer os.RemoveAll(tempDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)

	defer os.Setenv("HOME", originalHome)

	viper.Reset()
	os.Unsetenv("DATAROBOT_ENDPOINT")
	os.Unsetenv("DATAROBOT_API_TOKEN")
	viper.Set(config.DataRobotURL, "")
	baseURL := config.GetBaseURL()
	assert.Empty(t, baseURL, "Expected GetBaseURL to return empty string")

	result := EnsureAuthenticated()
	assert.False(t, result, "Expected EnsureAuthenticated to return false without configured URL")
}

func TestConfig_WriteAndRead(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	configFilePath := filepath.Join(os.Getenv("HOME"), ".config", "datarobot", "drconfig.yaml")
	viper.SetConfigFile(configFilePath)

	viper.Set(config.DataRobotAPIKey, "test-token")

	// SafeWriteConfig creates the file if it doesn't exist.
	err := viper.SafeWriteConfig()
	if err != nil {
		// File might already exist, try WriteConfig.
		err = viper.WriteConfig()
		require.NoError(t, err)
	}

	viper.Reset()
	viper.SetConfigFile(configFilePath)

	err = viper.ReadInConfig()
	require.NoError(t, err)

	token := viper.GetString(config.DataRobotAPIKey)
	assert.Equal(t, "test-token", token, "Expected token to be persisted and read back")
}

func TestConfig_ConfigFilePath(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "auth-test-*")
	require.NoError(t, err)

	defer os.RemoveAll(tempDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)

	defer os.Setenv("HOME", originalHome)

	viper.Reset()

	err = config.CreateConfigFileDirIfNotExists()
	require.NoError(t, err)

	// Verify config file was created in correct location.
	expectedPath := filepath.Join(tempDir, ".config", "datarobot", "drconfig.yaml")
	_, err = os.Stat(expectedPath)
	assert.NoError(t, err, "Expected config file to exist at %s", expectedPath)
}
