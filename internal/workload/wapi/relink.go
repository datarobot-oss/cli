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
// LastBuiltVersionID is dropped rather than carried, which leaves imageStale
// with nil and costs the next deploy a rebuild. That is the answer, not a
// side effect: the id records a code version in the catalog of the artifact
// being left, so comparing it against whatever the new artifact is serving
// compares two different lineages. imageStale already treats every answer it
// cannot establish as stale, and this is one of them.
//
// CreatedAt is kept. It records when this directory became a linked project,
// which re-pointing it does not change. CLIVersion is stamped fresh, because it
// records which build last wrote the file, and this write is the one that did.
//
// Those six are the whole of Config. A field added to it later has to be
// settled here by name too: this literal is the file after the write, so a
// field left out of it is a field cleared, whether or not that was meant.
//
// It is therefore not idempotent, and callers must not hand it the artifact the
// project is already linked to. Resetting the baseline is the whole point when
// the artifact changes and pure loss when it does not: with BASE empty and
// LastSyncedVersionID cleared, sync classifies a file that exists on both sides
// with different bytes as ADD_CONFLICT rather than LOCAL_MODIFIED, so the next
// run renames the local edit to <path>.LOCAL.<timestamp> and downloads the
// remote copy over it — an edit that would otherwise have uploaded cleanly.
// The check belongs to the caller because only it knows whether being handed
// the current id is a mistake or a deliberate baseline reset; the one caller
// there is, `artifact code init --force`, treats it as the former and no-ops.
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
