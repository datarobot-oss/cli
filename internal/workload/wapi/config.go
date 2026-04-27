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
// value round-trips as JSON null rather than "" (see design spec §3.1).
type Config struct {
	ArtifactID          string    `json:"artifactId"`
	CatalogID           *string   `json:"catalogId"`
	LastSyncedVersionID *string   `json:"lastSyncedVersionId"`
	CreatedAt           time.Time `json:"createdAt"`
	CLIVersion          string    `json:"cliVersion"`
}

// LoadConfig reads and parses .wapi/config.json. Returns ErrNotInitialized if
// .wapi/ (or config.json inside it) is missing, and a *CorruptedError
// wrapping the parse failure if the file is unreadable or malformed.
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

	return cfg, nil
}

// SaveConfig atomically writes c to .wapi/config.json. Returns
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

// writeConfig writes c to .wapi/config.json. Shared by SaveConfig (which
// maps os.ErrNotExist → ErrNotInitialized) and by Initialize (which has
// just mkdir'd .wapi/, so that error can't occur).
func writeConfig(projectDir string, c Config) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return atomicWriteFile(configPath(projectDir), data)
}

// stringPtr returns a pointer to v, or nil if v is empty. Lets callers
// construct Config values where optional fields serialize as JSON null when
// unset.
func stringPtr(v string) *string {
	if v == "" {
		return nil
	}

	return &v
}
