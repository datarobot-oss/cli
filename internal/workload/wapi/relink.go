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
	"fmt"
	"time"

	"github.com/datarobot/cli/internal/version"
)

// Relink points an already-linked project at a different artifact, in place.
//
// It exists so that "which artifact does this directory push to" has an answer
// that is a command. Before it, the only way to change one was to delete the
// state directory, which every message that needed to say so duly recommended —
// and which takes the project's ignore file and its whole history with it, for
// a change to one field.
//
// What it does not do is pretend the old artifact's code store still applies.
// CatalogID and LastSyncedVersionID come from the artifact being linked to, the
// same way Initialize takes them from a fresh one, because they describe that
// artifact's code store rather than this directory's history with the last one.
// Carrying them across would have the next sync diff against a catalog the new
// artifact never had, and report every file as unchanged when none of them are
// there.
//
// The BASE manifest goes back to empty for the same reason: it records the tree
// last synced into the artifact being left behind.
//
// CreatedAt is kept. It records when this directory became a linked project,
// which re-pointing it does not change. CLIVersion is stamped fresh, because it
// records which build last wrote the file, and this write is the one that did.
// Those two and the three above are the whole of Config: nothing is dropped by
// omission here, and a field added to Config later has to be settled here too.
//
// Returns ErrNotInitialized when there is no link to move. Relinking is not a
// way to create one: Initialize is, and it can say what a fresh link needs.
func Relink(projectDir string, opts InitOptions) error {
	if err := validateInitOptions(opts); err != nil {
		return err
	}

	if !Exists(projectDir) {
		return ErrNotInitialized
	}

	// Read before write, so a corrupted config is reported as itself rather
	// than silently replaced by this one. It also carries CreatedAt across.
	previous, err := LoadConfig(projectDir)
	if err != nil {
		return err
	}

	now := time.Now().UTC()

	cfg := Config{
		ArtifactID:          opts.ArtifactID,
		CatalogID:           stringPtr(opts.CatalogID),
		LastSyncedVersionID: stringPtr(opts.LastSyncedVersionID),
		CreatedAt:           previous.CreatedAt,
		CLIVersion:          version.Version,
	}

	if err := writeConfig(projectDir, cfg); err != nil {
		return fmt.Errorf("point %s at artifact %s: %w", projectDir, opts.ArtifactID, err)
	}

	if err := writeManifest(projectDir, Manifest{Version: ManifestVersion}); err != nil {
		return err
	}

	return AppendHistory(projectDir, HistoryEntry{
		"ts":       now.Format(time.RFC3339),
		"op":       "relink",
		"artifact": opts.ArtifactID,
		"from":     previous.ArtifactID,
		"catalog":  stringPtr(opts.CatalogID),
		"duration": "0.0s",
	})
}
