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
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/version"
)

// CommonProperties holds the set of properties attached to every telemetry
// event. These are collected once per CLI invocation and reused across all
// events in that session.
type CommonProperties struct {
	// TODO CFX-5206 figure out proper SessionID
	SessionID string // UUID v4, unique per process invocation
	DeviceID  string // UUID v4, stable per installation, persisted to disk
	// TODO CFX-5206 figure out proper UserID
	UserID            string // Placeholder for future user ID implementation
	CLIVersion        string // CLI version from version.Version (ldflags)
	InstallMethod     string // Build distribution method (ldflags)
	OSInfo            string // runtime.GOOS/runtime.GOARCH
	Environment       string // US, EU, JP, or custom — from endpoint URL
	DataRobotInstance string // Base URL of configured DataRobot instance
	CommandKind       string // "core" or "plugin", set by the root command after dispatch
}

// CollectCommonProperties gathers all common telemetry properties from the
// current environment. There are currently no network calls here, but we
// may want to add some in the future (e.g., to get user ID from DR API),
// so this function returns an error if any property collection step fails.
func CollectCommonProperties() *CommonProperties {
	props := &CommonProperties{
		SessionID:     generateSessionID(),
		DeviceID:      getOrCreateDeviceID(),
		CLIVersion:    version.Version,
		InstallMethod: InstallMethod,
		OSInfo:        runtime.GOOS + "/" + runtime.GOARCH,
	}

	// Get DataRobot instance info from config
	if endpoint := viperx.GetString(config.DataRobotURL); endpoint != "" {
		if baseURL, err := config.SchemeHostOnly(endpoint); err == nil {
			props.DataRobotInstance = baseURL
			props.Environment = deriveEnvironment(baseURL)
		}
	}

	// Get user ID (currently returns placeholder value)
	// TODO CFX-5206 implement proper user ID retrieval and consider privacy implications
	// Additionally, Amplitude strongly suggests not setting user ID until we
	// absolutely need it, and to not set the same user ID for anon users.

	// if userID, err := drapi.GetUserID(context.Background()); err == nil {
	// 	props.UserID = userID
	// }

	return props
}

// AsMap returns the properties as a map[string]interface{} suitable for
// merging into Amplitude event properties.
func (p *CommonProperties) AsMap() map[string]interface{} {
	return map[string]interface{}{
		"session_id":         p.SessionID,
		"cli_version":        p.CLIVersion,
		"install_method":     p.InstallMethod,
		"os_info":            p.OSInfo,
		"environment":        p.Environment,
		"datarobot_instance": p.DataRobotInstance,
		"command_kind":       p.CommandKind,
	}
}

// generateSessionID generates a UUID v4 for the current CLI session.
// This value is not persisted and will be different on each invocation.
// The default implementation uses crypto/rand, but if that fails, then
// fallback to a timestamp-based ID with a "fallback-" prefix to
// indicate it's not a true UUID.
func generateSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if crypto random generation fails
		return deviceIDFallbackPrefix + time.Now().UTC().Format(time.RFC3339)
	}

	// Set version (4) and variant (RFC 4122) bits
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return hex.EncodeToString(b)
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
	deviceIDFileName       = "device_id"
	deviceIDFallbackPrefix = "fallback-"
)

// getOrCreateDeviceID returns a stable device identifier.
func getOrCreateDeviceID() string {
	// First try to get machine ID from OS
	if id := getMachineID(); id != "" {
		return id
	}

	// Try to read existing device ID from file in the config directory
	configDir, err := config.GetConfigDir()
	if err != nil {
		// If we can't get the config directory, we won't be able to persist a device ID,
		// so we just generate a new one for this session. These IDs will be prefixed with
		// deviceIDFallbackPrefix to indicate it is not a true device ID.
		return deviceIDFallbackPrefix + generateSessionID()
	}

	// Try to read existing device ID from file
	deviceIDPath := filepath.Join(configDir, deviceIDFileName)

	data, err := os.ReadFile(deviceIDPath)
	if err == nil {
		// If we successfully read a device ID from the file, use it (after trimming whitespace).
		id := strings.TrimSpace(string(data))

		if id != "" {
			// If the ID is not empty, return it. Otherwise, we'll generate a new one below.
			return id
		}
	}

	// If we couldn't get a machine ID or read an existing device ID, generate a new one
	// and save it for future sessions. NOTE: Ignore errors at this point, since we can
	// still function without persisting.
	id := deviceIDFallbackPrefix + generateSessionID()

	// At this point, ignore any errors we might have with persisting the device ID, as
	// telemetry will still function without it, it will just be less stable.
	if mkErr := os.MkdirAll(configDir, 0o700); mkErr == nil {
		_ = os.WriteFile(deviceIDPath, []byte(id), 0o600)
	}

	return id
}
