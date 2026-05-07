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
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/log"
)

// readTextCacheFile reads a text file from the config directory, trimming whitespace.
// Returns empty string if the file doesn't exist or cannot be read.
func readTextCacheFile(filename string) string {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return ""
	}

	data, err := os.ReadFile(filepath.Join(configDir, filename))
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}

// writeTextCacheFile writes a text value to a file in the config directory.
// Ignores errors and returns gracefully if the write fails.
func writeTextCacheFile(filename, value string) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return
	}

	if mkErr := os.MkdirAll(configDir, 0o700); mkErr != nil {
		return
	}

	_ = os.WriteFile(filepath.Join(configDir, filename), []byte(value), 0o600)
}

// readJSONCacheFile reads and unmarshals a JSON file from the config directory.
// Returns an error if the file doesn't exist, cannot be read, or fails to unmarshal.
func readJSONCacheFile(filename string, v any) error {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(filepath.Join(configDir, filename))
	if err != nil {
		return err
	}

	return json.Unmarshal(data, v)
}

// writeJSONCacheFile marshals a value to JSON and writes it to a file in the config directory.
// Ignores errors and returns gracefully if the write fails.
func writeJSONCacheFile(filename string, v any) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		log.Debugf("Failed to get config directory for writing JSON cache file: %v", err)
		return
	}

	data, err := json.Marshal(v)
	if err != nil {
		log.Debugf("Failed to marshal JSON for cache file %s: %v", filename, err)
		return
	}

	if mkErr := os.MkdirAll(configDir, 0o700); mkErr != nil {
		log.Debugf("Failed to create config directory for JSON cache file %s: %v", filename, mkErr)
		return
	}

	_ = os.WriteFile(filepath.Join(configDir, filename), data, 0o600)
}
