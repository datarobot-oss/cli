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

// FileMeta is the per-file entry in the BASE manifest. Hash is SHA-256 hex
// (64 chars) as produced by the sync engine; this package does not validate
// its shape.
type FileMeta struct {
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

// Manifest is the parsed representation of .wapi/manifest.json — the BASE
// snapshot of "what both sides looked like the last time we successfully
// synced." Written by Initialize (empty) and by the sync engine.
//
// SyncedAt and SyncedVersionID use pointer semantics so that a freshly-init'd
// empty manifest round-trips with explicit JSON null values; the manifest is
// redundant with config.json so corruption of one is recoverable from the
// other.
type Manifest struct {
	Version         int                 `json:"version"`
	SyncedAt        *time.Time          `json:"syncedAt"`
	SyncedVersionID *string             `json:"syncedVersionId"`
	Files           map[string]FileMeta `json:"files"`
}

// LoadManifest reads and parses .wapi/manifest.json. Returns
// ErrNotInitialized if .wapi/ (or manifest.json inside it) is missing, and
// a *CorruptedError wrapping the parse failure if the file is unreadable or
// malformed.
func LoadManifest(projectDir string) (Manifest, error) {
	path := manifestPath(projectDir)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Manifest{}, ErrNotInitialized
		}

		return Manifest{}, &CorruptedError{Path: path, Err: err}
	}

	var m Manifest

	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, &CorruptedError{Path: path, Err: err}
	}

	if m.Files == nil {
		m.Files = map[string]FileMeta{}
	}

	return m, nil
}

// SaveManifest atomically writes the manifest to .wapi/manifest.json. Returns
// ErrNotInitialized if .wapi/ does not exist.
func SaveManifest(projectDir string, m Manifest) error {
	if err := writeManifest(projectDir, m); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotInitialized
		}

		return err
	}

	return nil
}

// writeManifest writes the manifest to .wapi/manifest.json. Shared by SaveManifest
// (which maps os.ErrNotExist → ErrNotInitialized) and by Initialize.
func writeManifest(projectDir string, m Manifest) error {
	// Ensure JSON emits "files": {} rather than "files": null.
	if m.Files == nil {
		m.Files = map[string]FileMeta{}
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	return atomicWriteFile(manifestPath(projectDir), data)
}
