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

package doctor

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"time"

	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/fsutil"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/datarobot/cli/internal/workload/wapi"
)

// Skip reasons shared by the repair operations. The sync-in-progress reason
// is part of the --fix output contract (every gated repair carries it), so
// it must not be reworded.
const (
	// ReasonSyncInProgress is the skip reason every repair reports when the
	// global safety gate finds a live process holding the sync lock. A sync
	// writes manifest.json in its final phase, so no repair is safe under it.
	ReasonSyncInProgress = "sync in progress: another process holds the sync lock"

	// ReasonLockUninspectable is the skip reason every repair reports when
	// the lock cannot be inspected: a hidden live sync cannot be ruled out.
	ReasonLockUninspectable = "cannot inspect the sync lock; a sync may be in progress, so no repair is safe"

	// ReasonNoLinkedState is the skip reason for repairs that need linked
	// state (a readable config) when the project has none.
	ReasonNoLinkedState = "no linked state; run 'dr artifact code init <artifact-id>' first"
)

// RunFix executes the `doctor --fix` repair suite for projectDir and returns
// one action per repair in the pinned order: manifest rebuild, rollback
// clear, lock clear.
//
// Global safety gate: the sync lock is probed first (non-creating probe,
// same logic as the wapi.lock check). When a live process holds the lock —
// or it cannot be inspected — ALL repairs are skipped with a reason, because
// a sync writes manifest.json in its final phase and must never be repaired
// underneath. --fix never touches the server; every write here is local.
func RunFix(ctx context.Context, projectDir string) []core.Action {
	return runFixWithGoos(ctx, projectDir, runtime.GOOS)
}

// runFixWithGoos is RunFix with the platform seam injected, so the windows
// gate path (flock not enforced) stays unit-testable on any host.
func runFixWithGoos(ctx context.Context, projectDir, goos string) []core.Action {
	switch gate := newLockCheckWithGoos(projectDir, goos).Run(ctx); gate.Status {
	case core.StatusFAIL:
		return skipAllRepairs(ReasonSyncInProgress)
	case core.StatusWARN:
		return skipAllRepairs(ReasonLockUninspectable)
	case core.StatusSKIP:
		// Windows (flock is not enforced there, per RAPTOR-16928) or a
		// project with no linked state: no live holder can exist or be
		// detected, so the gate lets the repairs through.
	case core.StatusOK:
		// Nothing held (or no lock file at all): the gate is open.
	}

	return []core.Action{
		fixManifest(projectDir),
		fixRollback(projectDir),
		fixLock(projectDir),
	}
}

// skipAllRepairs reports every repair as skipped with the given reason while
// the global safety gate blocks the run.
func skipAllRepairs(reason string) []core.Action {
	ids := []string{CheckIDManifest, CheckIDRollback, CheckIDLock}

	actions := make([]core.Action, 0, len(ids))

	for _, id := range ids {
		actions = append(actions, core.Action{ID: id, Status: core.ActionSkipped, Reason: reason})
	}

	return actions
}

// fixManifest rebuilds manifest.json as an empty BASE derived from config:
// Manifest{Version: 1, SyncedAt/SyncedVersionID nil-iff-config-nil,
// SyncedVersionID: cfg.LastSyncedVersionID, Files: {}}. The working tree is
// never touched. It requires a valid config: a corrupt config cannot name
// what the manifest should say, so the repair is skipped with a re-init
// remedy.
func fixManifest(projectDir string) core.Action {
	cfg, err := wapi.LoadConfig(projectDir)
	if err != nil {
		if errors.Is(err, wapi.ErrNotInitialized) {
			return core.Action{ID: CheckIDManifest, Status: core.ActionSkipped, Reason: ReasonNoLinkedState}
		}

		return core.Action{
			ID:     CheckIDManifest,
			Status: core.ActionSkipped,
			Reason: fmt.Sprintf(
				"config.json is corrupt or invalid (%s); the manifest cannot be rebuilt — re-initialize with 'dr artifact code init <artifact-id>'",
				corruptReason(err),
			),
		}
	}

	manifest, err := wapi.LoadManifest(projectDir)

	needsRebuild := err != nil || manifestDivergesFromConfig(cfg, manifest)

	if !needsRebuild {
		return core.Action{ID: CheckIDManifest, Status: core.ActionNotNeeded}
	}

	rebuilt := wapi.Manifest{
		Version: wapi.ManifestVersion,
		Files:   map[string]wapi.FileMeta{},
	}

	// Both-or-neither: the synced pointers are only written as a pair, so a
	// config with a last-synced version yields a manifest with both set.
	if versionID := normalizeStringPtr(cfg.LastSyncedVersionID); versionID != nil {
		now := time.Now().UTC()

		rebuilt.SyncedAt = &now

		rebuilt.SyncedVersionID = versionID
	}

	if err := wapi.SaveManifest(projectDir, rebuilt); err != nil {
		return core.Action{
			ID:     CheckIDManifest,
			Status: core.ActionSkipped,
			Reason: fmt.Sprintf("write rebuilt manifest: %v", err),
		}
	}

	return core.Action{
		ID:     CheckIDManifest,
		Status: core.ActionPerformed,
		Reason: "rebuilt manifest.json as an empty BASE from config.json; the next sync re-establishes the baseline",
	}
}

// manifestDivergesFromConfig reports whether the loaded manifest disagrees
// with the config's last-synced pointer (empty ≈ nil normalized). A missing
// or corrupt manifest is handled by the caller before this is consulted.
func manifestDivergesFromConfig(cfg wapi.Config, manifest wapi.Manifest) bool {
	return !pointersAgree(
		normalizeStringPtr(cfg.LastSyncedVersionID),
		normalizeStringPtr(manifest.SyncedVersionID),
	)
}

// fixRollback clears an interrupted rollback by restoring the backed-up
// files to the working tree and removing the .rollback/ tree(s). With no
// rollback tree present it reports not-needed.
func fixRollback(projectDir string) core.Action {
	present := false

	for _, dir := range wapi.StaleRollbackDirs(projectDir) {
		if fsutil.DirExists(dir) {
			present = true

			break
		}
	}

	if !present {
		return core.Action{ID: CheckIDRollback, Status: core.ActionNotNeeded}
	}

	restored, err := sync.RestoreStaleIfPresent(projectDir)
	if err != nil {
		return core.Action{
			ID:     CheckIDRollback,
			Status: core.ActionSkipped,
			Reason: fmt.Sprintf("restore interrupted rollback: %v", err),
		}
	}

	if !restored {
		// The tree vanished between the existence probe and the restore
		// (e.g. a racing cleanup). Nothing was restored, so nothing was done.
		return core.Action{ID: CheckIDRollback, Status: core.ActionNotNeeded}
	}

	return core.Action{
		ID:     CheckIDRollback,
		Status: core.ActionPerformed,
		Reason: "restored backed-up files to the working tree and removed .rollback/",
	}
}

// fixLock verifies the sync lock is clearable and leaves it untouched. An
// absent lock file is not-needed (and is never created); a present lock that
// AcquireSyncLock acquires is immediately released again and reported
// not-needed with a "verified acquirable" reason — the OS already released an
// unheld flock, so the file is the healthy steady state and nothing needed
// clearing (it is also never unlinked, because another process may hold the
// open descriptor). A lock that cannot be acquired (a holder appeared after
// the safety gate, or the file is uninspectable) is left exactly as found and
// reported skipped.
//
// Benign TOCTOU: the stat-then-acquire sequence has a race window — a sync
// could start between the stat and the AcquireSyncLock call. This is harmless:
// if a sync acquires the lock in that window, AcquireSyncLock fails and the
// repair reports skipped (the lock is held); if the lock file is created by a
// starting sync after the stat found it absent, AcquireSyncLock succeeds and
// is released, which is fine because the sync's own flock is on a different
// file descriptor (flock is per-open-file-description, not per-path). In both
// cases the post-fix check suite reports the honest state.
func fixLock(projectDir string) core.Action {
	path := filepath.Join(wapi.Dir(projectDir), sync.LockFileName)

	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return core.Action{ID: CheckIDLock, Status: core.ActionNotNeeded}
		}

		return core.Action{
			ID:     CheckIDLock,
			Status: core.ActionSkipped,
			Reason: fmt.Sprintf("stat sync lock %s: %v", path, err),
		}
	}

	lock, err := sync.AcquireSyncLock(projectDir)
	if err != nil {
		return core.Action{
			ID:     CheckIDLock,
			Status: core.ActionSkipped,
			Reason: fmt.Sprintf("sync lock could not be acquired: %v", err),
		}
	}

	if err := lock.Release(); err != nil {
		return core.Action{
			ID:     CheckIDLock,
			Status: core.ActionSkipped,
			Reason: fmt.Sprintf("release sync lock: %v", err),
		}
	}

	return core.Action{
		ID:     CheckIDLock,
		Status: core.ActionNotNeeded,
		Reason: "verified acquirable (acquired and released); no holder detected",
	}
}
