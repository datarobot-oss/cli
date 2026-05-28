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

package wapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// Config is the parsed representation of .wapi/config.json — the identity and
// sync-state pointer for the project directory.
//
// CatalogID and LastSyncedVersionID use pointer semantics so that an empty
// value round-trips as JSON null rather than "".
type Config struct {
	ArtifactID          string    `json:"artifactId" validate:"required,dr_id"`
	CatalogID           *string   `json:"catalogId" validate:"omitempty,dr_nonempty_ptr,dr_id"`
	LastSyncedVersionID *string   `json:"lastSyncedVersionId" validate:"omitempty,dr_nonempty_ptr,dr_id"`
	CreatedAt           time.Time `json:"createdAt" validate:"required"`
	CLIVersion          string    `json:"cliVersion" validate:"required"`
}

// LoadConfig reads and parses .wapi/config.json. Returns ErrNotInitialized if
// .wapi/ (or config.json inside it) is missing, and a *CorruptedError
// wrapping parse or semantic validation failures if the file is unreadable,
// malformed, or invalid.
func LoadConfig(projectDir string) (Config, error) {
	path := configPath(projectDir)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, ErrNotInitialized
		}

		return Config{}, &CorruptedError{Path: path, Err: err}
	}

	var cfg Config

	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, &CorruptedError{Path: path, Err: err}
	}

	if err := validateConfig(cfg); err != nil {
		return Config{}, &CorruptedError{Path: path, Err: err}
	}

	return cfg, nil
}

// SaveConfig atomically writes the config to .wapi/config.json. Returns
// ErrNotInitialized if .wapi/ does not exist.
func SaveConfig(projectDir string, c Config) error {
	if err := writeConfig(projectDir, c); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotInitialized
		}

		return err
	}

	return nil
}

// writeConfig writes the config to .wapi/config.json. Shared by SaveConfig (which
// maps os.ErrNotExist → ErrNotInitialized) and by Initialize (which has
// just mkdir'd .wapi/, so that error can't occur).
func writeConfig(projectDir string, c Config) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return atomicWriteFile(configPath(projectDir), data)
}

// stringPtr lets callers express "absent" as nil so optional Config fields
// serialize as JSON null rather than "".
func stringPtr(v string) *string {
	if v == "" {
		return nil
	}

	return &v
}
