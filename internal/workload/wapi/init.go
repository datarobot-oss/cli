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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/datarobot/cli/internal/fsutil"
	"github.com/datarobot/cli/internal/version"
)

// InitOptions carries the caller-supplied values that Initialize persists
// to config.json and the "init" history entry. CatalogID and
// LastSyncedVersionID are empty when the artifact has no committed code yet;
// empty strings serialize as JSON null.
type InitOptions struct {
	ArtifactID          string `validate:"required,dr_id"`
	CatalogID           string `validate:"omitempty,dr_id"`
	LastSyncedVersionID string `validate:"omitempty,dr_id"`
}

// Initialize creates the state directory at projectDir and writes all the
// bootstrap files: config.json, manifest.json (empty BASE), .gitignore ("*"),
// and an "init" entry in history.log. It also drops the .wapiignore template
// at projectDir if the user has no .wapiignore yet.
//
// Basic input validation happens before any filesystem changes (including
// creating projectDir or running mkdir for the state directory).
//
// projectDir is created (with any missing parents) if it does not already
// exist, matching the convenience of `git init <newdir>`.
//
// Returns ErrAlreadyLinked if the project already has a state directory. State
// left at the old root location does not count as linked and does not block
// linking, since re-linking is how a project recovers from it; callers who
// want to mention that directory ask LegacyStateNotice. On partial failure
// after mkdir, the incomplete tree is left in place for the user to inspect or
// remove manually rather than attempting rollback.
func Initialize(projectDir string, opts InitOptions) error {
	if err := validateInitOptions(opts); err != nil {
		return err
	}

	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return fmt.Errorf("create project directory %s: %w", projectDir, err)
	}

	if err := createStateDir(projectDir); err != nil {
		return err
	}

	now := time.Now().UTC()

	cfg := Config{
		ArtifactID:          opts.ArtifactID,
		CatalogID:           stringPtr(opts.CatalogID),
		LastSyncedVersionID: stringPtr(opts.LastSyncedVersionID),
		CreatedAt:           now,
		CLIVersion:          version.Version,
	}

	if err := writeConfig(projectDir, cfg); err != nil {
		return err
	}

	if err := writeManifest(projectDir, Manifest{Version: ManifestVersion}); err != nil {
		return err
	}

	if err := fsutil.AtomicWriteFile(gitignorePath(projectDir), []byte(gitignoreContents)); err != nil {
		return err
	}

	if !fsutil.FileExists(wapiignorePath(projectDir)) {
		if err := fsutil.AtomicWriteFile(wapiignorePath(projectDir), wapiignoreTemplate); err != nil {
			return err
		}
	}

	return AppendHistory(projectDir, HistoryEntry{
		"ts":        now.Format(time.RFC3339),
		"op":        "init",
		"artifact":  opts.ArtifactID,
		"catalog":   stringPtr(opts.CatalogID),
		"baseFiles": 0,
		"duration":  "0.0s",
	})
}

// createStateDir makes the state directory for a fresh link. The mkdir is not
// MkdirAll so that an existing directory surfaces as ErrAlreadyLinked instead
// of silently adopting whatever is already there.
func createStateDir(projectDir string) error {
	stateDir := Dir(projectDir)

	if err := os.MkdirAll(filepath.Dir(stateDir), 0o755); err != nil {
		return fmt.Errorf("create %s directory: %w", RootDirName, err)
	}

	if err := os.Mkdir(stateDir, 0o755); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrAlreadyLinked
		}

		return fmt.Errorf("create %s directory: %w", stateDir, err)
	}

	return nil
}
