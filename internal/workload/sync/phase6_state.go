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

package sync

import (
	"fmt"
	"time"

	"github.com/datarobot/cli/internal/workload/wapi"
)

// phase6State writes the new BASE manifest, config, history entry, and
// discards the rollback. Failures here do NOT roll back Phase 5 since
// the remote has already advanced; the next sync will reconcile.
func phase6State(e *Engine) error {
	if e.plan == nil {
		return nil
	}

	now := e.nowFn().UTC()

	cfg := e.config

	if e.newCatalogID != "" {
		cid := e.newCatalogID
		cfg.CatalogID = &cid
	}

	versionForState := e.newVersionID
	if versionForState == "" {
		// Pull-only sync; persist the remote version observed in Phase 1.
		versionForState = e.remoteVer
	}

	if versionForState != "" {
		cfg.LastSyncedVersionID = &versionForState
	}

	if err := wapi.SaveConfig(e.projectDir, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	manifest := buildNewBaseManifest(e, versionForState, now)
	if err := wapi.SaveManifest(e.projectDir, manifest); err != nil {
		return fmt.Errorf("save manifest: %w", err)
	}

	if err := wapi.AppendHistory(e.projectDir, syncHistoryEntry(e, now)); err != nil {
		return fmt.Errorf("append history: %w", err)
	}

	if e.rollback != nil {
		_ = e.rollback.Discard()
		e.rollback = nil
	}

	e.config = cfg
	e.populateResult(versionForState)

	return nil
}

// buildNewBaseManifest computes NEW_BASE = REMOTE + uploads (local hashes)
// - deletes, with conflicts resolved as remote-wins.
func buildNewBaseManifest(e *Engine, syncedVersionID string, syncedAt time.Time) wapi.Manifest {
	files := make(map[string]wapi.FileMeta, len(e.remote))

	for path, fe := range e.remote {
		files[path] = wapi.FileMeta{Hash: fe.Hash, Size: fe.Size}
	}

	for _, fa := range e.plan.Uploads {
		files[fa.Path] = wapi.FileMeta{Hash: fa.LocalHash, Size: fa.LocalSize}
	}

	for _, fa := range e.plan.Deletes {
		delete(files, fa.Path)
	}

	for _, fa := range e.plan.Conflicts {
		if fa.RemoteHash == "" {
			continue
		}

		files[fa.Path] = wapi.FileMeta{Hash: fa.RemoteHash, Size: fa.RemoteSize}
	}

	syncedAtCopy := syncedAt
	versionCopy := syncedVersionID

	return wapi.Manifest{
		Version:         wapi.ManifestVersion,
		SyncedAt:        &syncedAtCopy,
		SyncedVersionID: &versionCopy,
		Files:           files,
	}
}

// syncHistoryEntry assembles the JSONL line written to .wapi/history.log.
func syncHistoryEntry(e *Engine, now time.Time) wapi.HistoryEntry {
	entry := wapi.HistoryEntry{
		"ts":         now.Format(time.RFC3339),
		"op":         "sync",
		"version":    fmt.Sprintf("%s→%s", ShortVer(e.plan.OldVersionShort), ShortVer(e.newVersionID)),
		"uploaded":   len(e.plan.Uploads),
		"downloaded": len(e.plan.Downloads),
		"deleted":    len(e.plan.Deletes),
		"conflicts":  len(e.plan.Conflicts),
		"duration":   e.nowFn().Sub(e.startedAt).Round(time.Millisecond).String(),
	}

	if len(e.plan.Conflicts) > 0 {
		entry["conflict_files"] = e.plan.ConflictPaths()
	}

	return entry
}

func (e *Engine) populateResult(versionForState string) {
	r := &Result{
		OldVersion:      ptrOrEmpty(e.config.LastSyncedVersionID),
		NewVersion:      versionForState,
		UploadedCount:   len(e.plan.Uploads),
		DownloadedCount: len(e.plan.Downloads),
		DeletedCount:    len(e.plan.Deletes),
		ConflictCount:   len(e.plan.Conflicts),
		ConflictCopies:  e.conflictCopies,
		Duration:        e.nowFn().Sub(e.startedAt),
	}

	// "Old" should be the version BEFORE Phase 6 overwrote config.
	r.OldVersion = e.plan.OldVersionShort

	e.result = r
}
