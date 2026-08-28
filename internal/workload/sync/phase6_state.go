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

// phase6State writes the new BASE manifest, config, and history entry, and
// discards the rollback at entry. Failures here do NOT roll back Phase 5
// since the remote has already advanced; the next sync will reconcile. A
// Discard failure is the one entry failure that leaves the rollback dir
// behind, and it aborts before any state write so that leftover dir pairs
// with un-advanced state — the safe mid-Phase-5 shape (see below).
func phase6State(e *Engine) error {
	// Discard the rollback BEFORE any state write, unconditionally. When this
	// runs, Phase 5 executed the whole plan successfully (e.rollback is only
	// assigned after executePlan returns nil; a Phase 5 failure restores and
	// returns without reaching here), so the backup tree has no remaining
	// purpose — and Phase 6 never restores on failure because the remote has
	// already advanced. Discarding first makes the cleanup independent of
	// write success: if SaveManifest or SaveConfig below fails, the early
	// return must not strand the rollback dir, because the next run's
	// stale-rollback recovery would blindly copy the pre-sync bytes back
	// into the working tree. Against the manifest this run just wrote, those
	// resurrected bytes look like local edits and are silently re-uploaded
	// over the remote.
	if err := discardRollback(e); err != nil {
		return err
	}

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

	// Build and write the manifest BEFORE writing config. Both orders leave
	// a one-file window on failure, and the safe direction is the one where
	// the next sync detects drift and rebuilds from real remote data:
	//
	//   - SaveManifest fails: config has not been advanced yet, so the next
	//     sync sees the old version in config, detects drift, fetches
	//     AllFiles, and rebuilds BASE from the remote — safe and
	//     self-healing. The manifest write is retried by that same sync.
	//
	//   - SaveConfig fails: the manifest is already advanced while config
	//     still names the old version. The version mismatch makes the next
	//     sync detect drift and fetch the remote; BASE (the advanced
	//     manifest) truthfully describes that remote, so the plan is empty
	//     and the run merely converges config. The rollback dir is already
	//     gone by then — discarded at entry above — so no stale-restore can
	//     resurrect pre-sync bytes as false local edits.
	//
	// The converse (config advanced, manifest stale) is the poisonous
	// direction: the next sync sees no drift, fast-paths, copies the stale
	// BASE to REMOTE, and reports "Up to date." forever.
	//
	// This is data-safe because nothing between the two writes reads config
	// from disk. buildNewBaseManifest reads only e.remote, e.plan, and
	// e.uploadOutcome (all in-memory). e.config and populateResult are
	// touched only after both writes complete.
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

	if err := wapi.AppendHistory(e.projectDir, syncHistoryEntry(e, versionForState, now)); err != nil {
		return fmt.Errorf("append history: %w", err)
	}

	e.config = cfg
	e.populateResult(versionForState)

	return nil
}

// discardRollback removes the rollback tree at Phase 6 entry. A Discard
// failure must abort the phase here, before any state write: nothing is
// persisted yet, so returning an error leaves the next run with the rollback
// dir AND un-advanced state — the recoverable mid-Phase-5 outcome. The stale
// restore puts back bytes the un-advanced manifest still matches, and the
// next diff schedules downloads, not false uploads. Swallowing the error
// instead strands the rollback dir next to advanced state, where the same
// stale restore resurrects pre-sync bytes as phantom local edits which the
// next sync silently re-uploads over the remote.
func discardRollback(e *Engine) error {
	if e.rollback == nil {
		return nil
	}

	if err := e.rollback.Discard(); err != nil {
		return fmt.Errorf("discard rollback: %w", err)
	}

	e.rollback = nil

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

// syncHistoryEntry assembles the JSONL line written to history.log. The
// destination version is the one the run ended on (resolved in phase6State),
// not the raw new-version ID: a sync that applies no uploads — a pull-only
// sync, or the empty-plan state repair a --verify run performs — ends at the
// version it started from, and an empty tail would read as a truncated
// entry rather than as "no new version".
func syncHistoryEntry(e *Engine, versionForState string, now time.Time) wapi.HistoryEntry {
	entry := wapi.HistoryEntry{
		"ts":         now.Format(time.RFC3339),
		"op":         "sync",
		"version":    fmt.Sprintf("%s→%s", ShortVer(e.plan.OldVersionShort), ShortVer(versionForState)),
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
