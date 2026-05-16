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
	"context"
	"crypto/rand"
	"encoding/hex"
	"runtime"
	"strings"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/shell"
	"github.com/datarobot/cli/internal/version"
)

// CommonProperties holds the set of properties attached to every telemetry
// event. These are collected once per CLI invocation and reused across all
// events in that session.
type CommonProperties struct {
	// NOTE: When you add a new property here,
	// make sure to also add it to:
	// 1. AsMap() method
	// 2. CollectCommonProperties() function

	// top-level fields
	SessionID int64   // Unix ms timestamp, unique per process invocation
	DeviceID  string  // UUID v4, stable per installation, cached to disk
	UserID    *string // DataRobot uid from GET /api/v2/account/info/, cached to disk; nil on network failure or auth issues
	// event properties
	CLIVersion        string  // CLI version from version.Version (ldflags)
	InstallMethod     string  // Build distribution method (ldflags)
	OSName            string  // human-readable OS name from runtime.GOOS
	OSArch            string  // CPU architecture from runtime.GOARCH
	OSVersion         string  // OS release version string, detected at startup
	Language          string  // user language from LANG env var (e.g. "en_US")
	GoVersion         string  // Go runtime version (e.g. "go1.26.2")
	Shell             string  // Name of the user's shell (e.g. "zsh", "bash", "powershell")
	Environment       string  // US, EU, JP, or custom — from endpoint URL
	DataRobotInstance string  // Base URL of configured DataRobot instance
	CommandKind       string  // "core" or "plugin", set by the root command after dispatch
	OrganizationID    *string // DataRobot org ID from GET /api/v2/account/info/, cached to disk; nil on network failure or auth issues
	TenantID          *string // DataRobot tenant ID from GET /api/v2/account/info/, cached to disk; nil if unavailable (legit absent for legacy/system accounts)
}

// DetectShell returns the name of the shell the CLI is running from.
// Delegates to shell.DetectShell() which inspects the parent process first
// (accurate even after exec sh/bash/etc.) and falls back to $SHELL / OS
// defaults. Returns "unknown" if the shell cannot be determined.
func DetectShell() string {
	if name, err := shell.DetectShell(); err == nil {
		return name
	}

	return "unknown"
}

// CollectCommonProperties gathers all common telemetry properties from the
// current environment. This function will return an error if any property
// collection step fails.
func CollectCommonProperties() *CommonProperties {
	props := &CommonProperties{
		SessionID:     generateSessionID(),
		DeviceID:      getOrCreateDeviceID(),
		CLIVersion:    version.Version,
		InstallMethod: InstallMethod,
		OSName:        humanizeOS(runtime.GOOS),
		OSArch:        runtime.GOARCH,
		OSVersion:     detectOSVersion(),
		Language:      detectLanguage(),
		GoVersion:     runtime.Version(),
		Shell:         DetectShell(),
	}

	// Get DataRobot instance info from config
	if endpoint := viperx.GetString(config.DataRobotURL); endpoint != "" {
		if baseURL, err := config.SchemeHostOnly(endpoint); err == nil {
			props.DataRobotInstance = baseURL
			props.Environment = deriveEnvironment(baseURL)
		}
	}

	// Retrieve account info (includes userID, orgID, tenantID)
	result, err := retrieveAccountInfo(context.Background())
	if err == nil {
		props.UserID = &result.UID
		props.OrganizationID = &result.OrganizationID

		if result.TenantID != "" {
			props.TenantID = &result.TenantID
		}
	}

	return props
}

// AsMap returns the properties as a map[string]any suitable for
// merging into Amplitude event properties. Note: UserID, DeviceID, and SessionID
// are not included here as they are set as top-level Amplitude event fields.
func (p *CommonProperties) AsMap() map[string]any {
	m := map[string]any{
		"install_method":     p.InstallMethod,
		"os_arch":            p.OSArch,
		"go_version":         p.GoVersion,
		"shell":              p.Shell,
		"environment":        p.Environment,
		"datarobot_instance": p.DataRobotInstance,
		"command_kind":       p.CommandKind,
	}

	if p.OrganizationID != nil {
		m["organization_id"] = *p.OrganizationID
	}

	if p.TenantID != nil {
		m["tenant_id"] = *p.TenantID
	}

	return m
}

// generateSessionID returns a Unix timestamp in milliseconds for the current
// CLI session. This value is not persisted and will be different on each
// invocation. Amplitude's top-level session_id field expects an integer value.
func generateSessionID() int64 {
	return time.Now().UnixMilli()
}

// deriveEnvironment determines the DataRobot environment (US/EU/JP/custom)
// from the endpoint URL.
// TODO Is this really necessary? Can we remove this and just report
// the base URL?
func deriveEnvironment(baseURL string) string {
	switch {
	case strings.Contains(baseURL, "app.datarobot.com"):
		return "US"
	case strings.Contains(baseURL, "app.eu.datarobot.com"):
		return "EU"
	case strings.Contains(baseURL, "app.jp.datarobot.com"):
		return "JP"
	default:
		return "custom"
	}
}

const (
	userIDFileName         = "user_id"
	deviceIDFileName       = "device_id"
	deviceIDFallbackPrefix = "fallback-"
)

// getOrCreateDeviceID returns a stable device identifier.
func getOrCreateDeviceID() string {
	// First try to get machine ID from OS
	if id := getMachineID(); id != "" {
		return id
	}

	// Try to read existing device ID from cache file
	if id := readTextCacheFile(deviceIDFileName); id != "" {
		return id
	}

	// If we couldn't get a machine ID or read an existing device ID, generate a new one
	// and save it for future sessions. NOTE: Ignore errors at this point, since we can
	// still function without persisting.
	// It might be worth considering supporting a DATAROBOT_CLI_DEVICE_ID environment
	// variable for environments where filesystem persistence is unreliable or machine
	// IDs are ephemeral (e.g., certain container orchestration systems). This way we
	// could intentionally pass in a stable ID. Honestly, though, I think being able
	// to write out a device_id file is good enough.
	id := deviceIDFallbackPrefix + randomHexID()

	writeTextCacheFile(deviceIDFileName, id)

	return id
}

// randomHexID generates a random 32-character hex string used as a device ID fallback.
func randomHexID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return time.Now().UTC().Format(time.RFC3339)
	}

	// Set version (4) and variant (RFC 4122) bits
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return hex.EncodeToString(b)
}
