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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/internal/version"
	"github.com/spf13/viper"
)

// CommonProperties holds the set of properties attached to every telemetry
// event. These are collected once per CLI invocation and reused across all
// events in that session.
type CommonProperties struct {
	// TODO CFX-5206 figure out proper SessionID
	SessionID string // UUID v4, unique per process invocation
	// TODO CFX-5206 figure out proper UserID
	UserID            string // Placeholder for future user ID implementation
	CLIVersion        string // CLI version from version.Version (ldflags)
	InstallMethod     string // Build distribution method (ldflags)
	OSInfo            string // runtime.GOOS/runtime.GOARCH
	Environment       string // US, EU, JP, or custom — from endpoint URL
	DataRobotInstance string // Base URL of configured DataRobot instance
	TemplateName      string // Best-effort from .datarobot/answers/ dir
}

// CollectCommonProperties gathers all common telemetry properties from the
// current environment. There are currently no network calls here, but we
// may want to add some in the future (e.g., to get user ID from DR API),
// so this function returns an error if any property collection step fails.
func CollectCommonProperties() *CommonProperties {
	props := &CommonProperties{
		SessionID:     generateSessionID(),
		CLIVersion:    version.Version,
		InstallMethod: InstallMethod,
		OSInfo:        runtime.GOOS + "/" + runtime.GOARCH,
	}

	// Get DataRobot instance info from config
	if endpoint := viper.GetString(config.DataRobotURL); endpoint != "" {
		if baseURL, err := config.SchemeHostOnly(endpoint); err == nil {
			props.DataRobotInstance = baseURL
			props.Environment = deriveEnvironment(baseURL)
		}
	}

	// Get user ID (currently returns placeholder value)
	if userID, err := drapi.GetUserID(context.Background()); err == nil {
		props.UserID = userID
	}

	// Get template name from repo
	if templateName, err := getTemplateName(); err == nil {
		props.TemplateName = templateName
	}

	return props
}

// AsMap returns the properties as a map[string]interface{} suitable for
// merging into Amplitude event properties.
func (p *CommonProperties) AsMap() map[string]interface{} {
	return map[string]interface{}{
		"session_id":         p.SessionID,
		"user_id":            p.UserID,
		"cli_version":        p.CLIVersion,
		"install_method":     p.InstallMethod,
		"os_info":            p.OSInfo,
		"environment":        p.Environment,
		"datarobot_instance": p.DataRobotInstance,
		"template_name":      p.TemplateName,
	}
}

// generateSessionID creates a UUID v4 for the session.
func generateSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if crypto fails
		return time.Now().Format("20060102150405") + "-fallback"
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

// getTemplateName attempts to extract the template name from the .datarobot/answers directory.
// Returns empty string if not in a DataRobot repo.
// TODO I think this could be moved to internal/repo and more robustly implemented.
func getTemplateName() (string, error) {
	repoRoot, err := repo.FindRepoRoot()
	if err != nil {
		return "", err
	}

	answersDir := filepath.Join(repoRoot, ".datarobot", "answers")

	entries, err := os.ReadDir(answersDir)
	if err != nil {
		return "", err
	}

	// Try to find the first YAML file that might indicate template name
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml") {
			// Extract template name from filename (e.g., "base.yml" -> "base")
			baseName := strings.TrimSuffix(name, filepath.Ext(name))
			if baseName != "" {
				return baseName, nil
			}
		}
	}

	return "", nil
}
