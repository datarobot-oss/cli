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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/state"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptrString(s string) *string {
	return &s
}

func TestGenerateSessionID_ReturnsValidTimestamp(t *testing.T) {
	before := time.Now().UnixMilli()
	id := generateSessionID()
	after := time.Now().UnixMilli()

	assert.GreaterOrEqual(t, id, before)
	assert.LessOrEqual(t, id, after)
}

func TestGenerateSessionID_UniqueSessions(t *testing.T) {
	id1 := generateSessionID()

	time.Sleep(2 * time.Millisecond)

	id2 := generateSessionID()

	assert.NotEqual(t, id1, id2, "session IDs should be unique")
}

func TestCommonPropertiesAsMap(t *testing.T) {
	props := &CommonProperties{
		SessionID:         1234567890,
		DeviceID:          "device-789",
		UserID:            ptrString("user-456"),
		CLIVersion:        "v0.1.0",
		InstallMethod:     "source",
		Shell:             "zsh",
		DataRobotInstance: "https://app.datarobot.com",
		CommandKind:       "core",
		OrganizationID:    ptrString("parakeet"),
		TenantID:          ptrString("parakeet-jones"),
	}

	m := props.AsMap()

	assert.Equal(t, "source", m["install_method"])
	assert.Equal(t, "zsh", m["shell"])
	assert.Equal(t, "https://app.datarobot.com", m["datarobot_instance"])
	assert.Equal(t, "core", m["command_kind"])
	assert.Equal(t, "parakeet", m["organization_id"])
	assert.Equal(t, "parakeet-jones", m["tenant_id"])
	// Verify CWD is not included
	assert.NotContains(t, m, "cwd")
	// session_id, user_id, and device_id are top-level Amplitude fields, not event properties
	assert.NotContains(t, m, "session_id")
	assert.NotContains(t, m, "user_id")
	// template_name is absent when not set
	assert.NotContains(t, m, "template_name")
}

func TestCommonPropertiesAsMap_IncludesTemplateName(t *testing.T) {
	props := &CommonProperties{
		TemplateName: ptrString("my-template"),
		TemplateID:   ptrString("tmpl-abc123"),
	}

	m := props.AsMap()

	assert.Equal(t, "my-template", m["template_name"])
	assert.Equal(t, "tmpl-abc123", m["template_id"])
}

func TestCommonPropertiesAsMap_OmitsTemplateNameWhenNil(t *testing.T) {
	props := &CommonProperties{}

	m := props.AsMap()

	assert.NotContains(t, m, "template_name")
	assert.NotContains(t, m, "template_id")
}

func TestGetOrCreateDeviceID_CreatesAndPersists(t *testing.T) {
	if getMachineID() != "" {
		t.Skip("OS machine ID available; file fallback not exercised")
	}

	tmpDir := t.TempDir()

	testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", tmpDir)

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

	testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", tmpDir)

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

	testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", tmpDir)

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

	testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", tmpDir)

	id := getOrCreateDeviceID()

	assert.NotEmpty(t, id)
	assert.Contains(t, id, "fallback-")
}

func TestCollectCommonProperties_SetsDeviceID(t *testing.T) {
	tmpDir := t.TempDir()

	testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", tmpDir)

	props := CollectCommonProperties()

	assert.NotEmpty(t, props.DeviceID)
}

func TestCollectCommonProperties_UserIDNilWhenUnauthenticated(t *testing.T) {
	tmpDir := t.TempDir()

	testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", tmpDir)

	defer viperx.Reset()

	props := CollectCommonProperties()

	assert.Nil(t, props.UserID)
	assert.Nil(t, props.OrganizationID)
	assert.Nil(t, props.TenantID)
}

func TestCollectCommonProperties_UserIDFromCacheWhenAuthenticated(t *testing.T) {
	tmpDir := t.TempDir()

	testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", tmpDir)

	defer viperx.Reset()

	viperx.Set(config.DataRobotURL, "https://test.datarobot.com/api/v2")
	viperx.Set(config.DataRobotAPIKey, "test-token")

	// Pre-seed the cache file with a valid entry matching the configured endpoint and token
	configDir := filepath.Join(tmpDir, "datarobot")

	err := os.MkdirAll(configDir, 0o700)

	require.NoError(t, err)

	token := "test-token"
	hash := sha256.Sum256([]byte(token))
	fingerprint := hex.EncodeToString(hash[:])

	cache := accountCache{
		UID:              "cross-test-uid",
		Endpoint:         "https://test.datarobot.com",
		TokenFingerprint: fingerprint,
		OrganizationID:   "parakeet",
		TenantID:         "parakeet-jones",
	}
	data, err := json.Marshal(cache)

	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(configDir, userIDFileName), data, 0o600)

	require.NoError(t, err)

	props := CollectCommonProperties()

	require.NotNil(t, props.UserID)
	assert.Equal(t, "cross-test-uid", *props.UserID)
	require.NotNil(t, props.OrganizationID)
	assert.Equal(t, "parakeet", *props.OrganizationID)
	require.NotNil(t, props.TenantID)
	assert.Equal(t, "parakeet-jones", *props.TenantID)

	m := props.AsMap()

	assert.NotContains(t, m, "user_id")
	assert.Equal(t, "parakeet", m["organization_id"])
	assert.Equal(t, "parakeet-jones", m["tenant_id"])
}

func TestCommonPropertiesAsMap_DefaultCommandKindIsEmpty(t *testing.T) {
	props := &CommonProperties{}

	m := props.AsMap()

	// CommandKind is set by the root command after dispatch; the freshly
	// collected properties default to an empty string.
	assert.Empty(t, m["command_kind"])
	assert.Contains(t, m, "command_kind")
}

func TestCollectCommonProperties_GeneratesSessionID(t *testing.T) {
	before := time.Now().UnixMilli()
	props := CollectCommonProperties()
	after := time.Now().UnixMilli()

	assert.GreaterOrEqual(t, props.SessionID, before)
	assert.LessOrEqual(t, props.SessionID, after)
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

func TestCollectCommonProperties_DefaultCommandKindIsEmpty(t *testing.T) {
	props := CollectCommonProperties()

	// CommandKind is intentionally not populated by CollectCommonProperties;
	// the root command sets it once it knows whether the dispatched command
	// is a core or plugin command.
	assert.Empty(t, props.CommandKind)
}

func TestCollectCommonProperties_SetsOSName(t *testing.T) {
	props := CollectCommonProperties()

	assert.NotEmpty(t, props.OSName)
}

func TestDetectLanguage_ReturnsNonEmpty(t *testing.T) {
	assert.NotEmpty(t, detectLanguage())
}

func TestCollectCommonProperties_SetsOSArch(t *testing.T) {
	props := CollectCommonProperties()

	assert.Equal(t, runtime.GOARCH, props.OSArch)
}

func TestCollectCommonProperties_DetectsShell(t *testing.T) {
	props := CollectCommonProperties()

	// Shell is detected via parent process name; in the test runner
	// (go/task) it will be non-empty and not "unknown".
	assert.NotEmpty(t, props.Shell)
}

func TestDetectShell_ReturnsNonEmpty(t *testing.T) {
	shell := DetectShell()

	assert.NotEmpty(t, shell)
	assert.NotEqual(t, "unknown", shell)
}

func TestCollectCommonProperties_TemplateNameFromState(t *testing.T) {
	tmpDir := t.TempDir()

	// Resolve symlinks (important for macOS where /var -> /private/var)
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	// Create .datarobot/answers to satisfy repo detection
	answersDir := filepath.Join(tmpDir, ".datarobot", "answers")
	err = os.MkdirAll(answersDir, 0o755)
	require.NoError(t, err)

	// Write state with template name and ID
	err = state.UpdateAfterTemplatesSetup(tmpDir, "telemetry-test-template", "tmpl-tst-001")
	require.NoError(t, err)

	// Change to temp dir so FindRepoRoot() finds it
	originalWd, err := os.Getwd()
	require.NoError(t, err)

	defer func() {
		require.NoError(t, os.Chdir(originalWd))
	}()

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", tmpDir)

	props := CollectCommonProperties()

	require.NotNil(t, props.TemplateName)
	assert.Equal(t, "telemetry-test-template", *props.TemplateName)
	require.NotNil(t, props.TemplateID)
	assert.Equal(t, "tmpl-tst-001", *props.TemplateID)

	m := props.AsMap()

	assert.Equal(t, "telemetry-test-template", m["template_name"])
	assert.Equal(t, "tmpl-tst-001", m["template_id"])
}

func TestCollectCommonProperties_TemplateNameNilOutsideProject(t *testing.T) {
	tmpDir := t.TempDir()

	// No .datarobot directory — not a template project
	originalWd, err := os.Getwd()
	require.NoError(t, err)

	defer func() {
		require.NoError(t, os.Chdir(originalWd))
	}()

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", tmpDir)

	props := CollectCommonProperties()

	assert.Nil(t, props.TemplateName)

	m := props.AsMap()

	assert.NotContains(t, m, "template_name")
}
