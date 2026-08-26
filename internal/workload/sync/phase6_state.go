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

	// Build and write the manifest BEFORE writing config. The failure
	// direction is what matters, not elegance: if SaveManifest fails,
	// config has not been advanced yet, so the next sync sees the old
	// version in config, detects drift, fetches AllFiles, and rebuilds
	// BASE from real remote data — safe and self-healing. The converse
	// (config advanced, manifest stale) silently poisons BASE: the next
	// sync sees no drift, fast-paths, copies the stale BASE to REMOTE,
	// and reports "Up to date." forever.
	//
	// This reorder is data-safe: nothing between the two writes reads
	// config from disk. buildNewBaseManifest reads only e.remote,
	// e.plan, and e.uploadOutcome (all in-memory). e.config and
	// populateResult are touched only after both writes complete.
	manifest, err := buildNewBaseManifest(e, versionForState, now)
	if err != nil {
		return fmt.Errorf("build manifest: %w", err)
	}

	if err := wapi.SaveManifest(e.projectDir, manifest); err != nil {
		return fmt.Errorf("save manifest: %w", err)
	}

	if err := wapi.SaveConfig(e.projectDir, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
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

// buildNewBaseManifest computes NEW_BASE = REMOTE + uploads (streamed hashes)
// - deletes, with conflicts resolved as remote-wins. Each uploaded path's
// hash and size come from the UploadOutcome recorded in Phase 5 — the bytes
// that actually crossed the wire, not the Phase-2 planned hash. A missing
// Sent entry is a hard error naming the path: a per-path fallback to the
// planned hash IS the original poisoning bug and must not exist here.
func buildNewBaseManifest(e *Engine, syncedVersionID string, syncedAt time.Time) (wapi.Manifest, error) {
	files := make(map[string]wapi.FileMeta, len(e.remote))

	for path, fe := range e.remote {
		files[path] = wapi.FileMeta{Hash: fe.Hash, Size: fe.Size}
	}

	for _, fa := range e.plan.Uploads {
		if e.uploadOutcome == nil {
			return wapi.Manifest{}, fmt.Errorf("internal: no upload outcome recorded for %s", fa.Path)
		}

		sent, ok := e.uploadOutcome.Sent[fa.Path]
		if !ok {
			// Refuse rather than fall back: phase6 overwrites unconditionally,
			// so a per-path fallback to fa.LocalHash silently reintroduces
			// the poisoning. Either every upload has a Sent entry, or the
			// sync fails.
			return wapi.Manifest{}, fmt.Errorf("internal: no streamed hash recorded for %s", fa.Path)
		}

		files[fa.Path] = wapi.FileMeta{Hash: sent.Hash, Size: sent.Size}
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
	}, nil
}

// syncHistoryEntry assembles the JSONL line written to history.log.
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
