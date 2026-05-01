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

package telemetry

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSessionID_ReturnsValidUUID(t *testing.T) {
	id := generateSessionID()

	// UUID v4 format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	uuidPattern := regexp.MustCompile(`^[0-9a-f]{32}$`)
	assert.True(t, uuidPattern.MatchString(id), "session ID should be 32 hex characters")
	assert.NotEmpty(t, id)
}

func TestGenerateSessionID_UniqueSessions(t *testing.T) {
	id1 := generateSessionID()
	id2 := generateSessionID()

	assert.NotEqual(t, id1, id2, "session IDs should be unique")
}

func TestCommonPropertiesAsMap(t *testing.T) {
	props := &CommonProperties{
		SessionID:         "session-123",
		DeviceID:          "device-789",
		UserID:            "user-456",
		CLIVersion:        "v0.1.0",
		InstallMethod:     "source",
		OSInfo:            "darwin/arm64",
		Environment:       "US",
		DataRobotInstance: "https://app.datarobot.com",
		TemplateName:      "base",
	}

	m := props.AsMap()

	assert.Equal(t, "session-123", m["session_id"])
	assert.Equal(t, "user-456", m["user_id"])
	assert.Equal(t, "v0.1.0", m["cli_version"])
	assert.Equal(t, "source", m["install_method"])
	assert.Equal(t, "darwin/arm64", m["os_info"])
	assert.Equal(t, "US", m["environment"])
	assert.Equal(t, "https://app.datarobot.com", m["datarobot_instance"])
	assert.Equal(t, "base", m["template_name"])
	// Verify CWD is not included
	assert.NotContains(t, m, "cwd")
}

func TestGetOrCreateDeviceID_CreatesAndPersists(t *testing.T) {
	if getMachineID() != "" {
		t.Skip("OS machine ID available; file fallback not exercised")
	}

	tmpDir := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	id1 := getOrCreateDeviceID()

	assert.NotEmpty(t, id1)

	// Second call should return the same ID
	id2 := getOrCreateDeviceID()

	assert.Equal(t, id1, id2)

	// File should exist
	deviceIDPath := filepath.Join(tmpDir, "datarobot", deviceIDFileName)
	data, err := os.ReadFile(deviceIDPath)

	require.NoError(t, err)
	assert.Equal(t, id1, string(data))
}

func TestGetOrCreateDeviceID_ReadsExistingID(t *testing.T) {
	if getMachineID() != "" {
		t.Skip("OS machine ID available; file fallback not exercised")
	}

	tmpDir := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	configDir := filepath.Join(tmpDir, "datarobot")

	err := os.MkdirAll(configDir, 0o700)

	require.NoError(t, err)

	existingID := "abcdef1234567890abcdef1234567890"

	err = os.WriteFile(filepath.Join(configDir, deviceIDFileName), []byte(existingID), 0o600)

	require.NoError(t, err)

	id := getOrCreateDeviceID()

	assert.Equal(t, existingID, id)
}

func TestGetOrCreateDeviceID_IgnoresBlankFile(t *testing.T) {
	if getMachineID() != "" {
		t.Skip("OS machine ID available; file fallback not exercised")
	}

	tmpDir := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	configDir := filepath.Join(tmpDir, "datarobot")

	err := os.MkdirAll(configDir, 0o700)

	require.NoError(t, err)

	// Write a blank file — should be treated as absent and regenerated
	err = os.WriteFile(filepath.Join(configDir, deviceIDFileName), []byte("   "), 0o600)

	require.NoError(t, err)

	id := getOrCreateDeviceID()

	assert.NotEmpty(t, id)
	assert.Contains(t, id, "fallback-", "blank file should trigger fallback ID generation")
}

func TestGetOrCreateDeviceID_FallbackIDHasPrefix(t *testing.T) {
	if getMachineID() != "" {
		t.Skip("OS machine ID available; file fallback not exercised")
	}

	tmpDir := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	id := getOrCreateDeviceID()

	assert.NotEmpty(t, id)
	assert.Contains(t, id, "fallback-")
}

func TestCollectCommonProperties_SetsDeviceID(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	props := CollectCommonProperties()

	assert.NotEmpty(t, props.DeviceID)
}

func TestDeriveEnvironment_US(t *testing.T) {
	assert.Equal(t, "US", deriveEnvironment("https://app.datarobot.com"))
}

func TestDeriveEnvironment_EU(t *testing.T) {
	assert.Equal(t, "EU", deriveEnvironment("https://app.eu.datarobot.com"))
}

func TestDeriveEnvironment_JP(t *testing.T) {
	assert.Equal(t, "JP", deriveEnvironment("https://app.jp.datarobot.com"))
}

func TestDeriveEnvironment_Custom(t *testing.T) {
	assert.Equal(t, "custom", deriveEnvironment("https://custom.internal.company.com"))
}

func TestCollectCommonProperties_ContainsOSInfo(t *testing.T) {
	props := CollectCommonProperties()

	assert.NotEmpty(t, props.OSInfo)
	assert.Contains(t, props.OSInfo, runtime.GOOS)
	assert.Contains(t, props.OSInfo, runtime.GOARCH)
}

func TestCollectCommonProperties_GeneratesSessionID(t *testing.T) {
	props := CollectCommonProperties()

	assert.NotEmpty(t, props.SessionID)
	assert.Len(t, props.SessionID, 32)
}

func TestCollectCommonProperties_SetsInstallMethod(t *testing.T) {
	props := CollectCommonProperties()

	// Should use the package-level variable
	assert.NotEmpty(t, props.InstallMethod)
}

func TestCollectCommonProperties_SetsCLIVersion(t *testing.T) {
	props := CollectCommonProperties()

	// Should be populated from version package
	assert.NotEmpty(t, props.CLIVersion)
}
