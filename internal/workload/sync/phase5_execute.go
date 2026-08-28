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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/datarobot/cli/internal/workload/fileops"
)

// phase5Execute applies the SyncPlan in the order: disk-space check +
// rollback dir, conflict copies, downloads + local-side deletes, remote
// deletes, uploads. Any error after the rollback dir is created triggers
// a Restore to pre-sync state.
func phase5Execute(e *Engine) error {
	// Above the empty-plan return, because this is a statement about the
	// artifact rather than about the plan: phase 6 writes local state and a
	// history entry for an empty plan too, and recording a sync against
	// something immutable is the same lie as performing one. Phase 1 lets a
	// preview plan against a locked artifact and Run returns before this phase
	// in both preview modes, but nothing stops a caller holding the engine from
	// planning and then executing anyway.
	if e.artifact != nil && e.artifact.IsLocked() {
		return fmt.Errorf("artifact %s is locked (immutable); refusing to execute a plan against it", e.artifact.ID)
	}

	if e.plan == nil || e.plan.IsEmpty() {
		return nil
	}

	// Reject server-controlled traversal paths before any filesystem op
	// runs, including Rollback.Backup which stats/reads the source path
	// and would otherwise touch files outside e.projectDir.
	if err := validateServerPaths(e.plan); err != nil {
		return err
	}

	if len(e.plan.Uploads)+len(e.plan.Downloads)+len(e.plan.Deletes)+len(e.plan.Conflicts) > RollbackMaxFiles {
		return fmt.Errorf("plan exceeds RollbackMaxFiles=%d; refusing to run", RollbackMaxFiles)
	}

	if err := EnsureSpaceFor(e.projectDir, e.plan.TotalDownloadBytes()); err != nil {
		return err
	}

	rb, err := NewRollback(e.projectDir)
	if err != nil {
		return err
	}

	if err := executePlan(e, rb); err != nil {
		_ = rb.Restore()
		return err
	}

	// Hand the rollback to Phase 6, which discards it at entry — before its
	// first state write, not after its last. Once the plan has executed
	// successfully the backup tree protects nothing, and discarding
	// unconditionally keeps a Phase 6 write failure from stranding the dir
	// for the next run's stale-restore to resurrect pre-sync bytes from.
	// A Phase 5 failure above restores and returns, leaving the dir in place
	// for exactly that stale-restore recovery.
	e.rollback = rb

	return nil
}

func executePlan(e *Engine, rb *Rollback) error {
	if err := applyConflictCopies(e, rb); err != nil {
		return fmt.Errorf("conflict copies: %w", err)
	}

	codeRef := codeRefOrEmpty(e)

	if err := applyDownloads(e, rb, codeRef); err != nil {
		return fmt.Errorf("downloads: %w", err)
	}

	if err := applyLocalDeletes(e, rb); err != nil {
		return fmt.Errorf("local deletes: %w", err)
	}

	newCatalogID, newVersionID, err := applyRemoteDeletesAndUploads(e, codeRef)
	if err != nil {
		return err
	}

	e.newCatalogID = newCatalogID
	e.newVersionID = newVersionID

	// --verify's post-apply check runs before this function returns so a
	// mismatch propagates to phase5Execute, which restores the working tree
	// and stops the pipeline before phase6State can persist anything. The
	// check itself is a no-op unless the plan uploaded files and Options.Verify
	// opted in.
	if err := verifyPostApplyUploads(e, e.uploadOutcome); err != nil {
		return err
	}

	return nil
}

// applyConflictCopies renames each conflict's local file to
// <path>.LOCAL.<ISO8601Z>. EDIT_DEL_CONFLICT is excluded since the user
// already deleted that file.
func applyConflictCopies(e *Engine, rb *Rollback) error {
	stamp := e.nowFn().UTC().Format("20060102T150405Z")

	for _, fa := range e.plan.Conflicts {
		if fa.Classification == ClsEditDelConflict {
			continue
		}

		src := filepath.Join(e.projectDir, filepath.FromSlash(fa.Path))
		dst := src + ".LOCAL." + stamp

		if err := rb.Backup(fa.Path); err != nil {
			return err
		}

		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("rename %s -> %s: %w", fa.Path, dst, err)
		}

		rb.TrackCreated(dst)
		e.conflictCopies = append(e.conflictCopies, dst)
	}

	return nil
}

// applyDownloads runs the download list plus the conflict-copy follow-up
// downloads (remote wins, so remote bytes land at the original path).
// REMOTE_DELETED files are removed locally instead.
func applyDownloads(e *Engine, rb *Rollback, codeRef codeRefRef) error {
	if codeRef.CatalogID == "" || codeRef.CatalogVersionID == "" {
		return nil
	}

	if err := backupDownloadTargets(e, rb); err != nil {
		return err
	}

	if err := removeLocalDeletedFiles(e); err != nil {
		return err
	}

	pulls := pullList(e.plan)
	if err := downloadFiles(e, codeRef.CatalogID, codeRef.CatalogVersionID, pulls); err != nil {
		return err
	}

	for _, fa := range pulls {
		rb.TrackCreated(filepath.Join(e.projectDir, filepath.FromSlash(fa.Path)))
	}

	return nil
}

func backupDownloadTargets(e *Engine, rb *Rollback) error {
	for _, fa := range e.plan.Downloads {
		if err := rb.Backup(fa.Path); err != nil {
			return err
		}
	}

	// ActDownloadDelete lives in plan.Deletes, so back those up here
	// before removeLocalDeletedFiles removes them.
	for _, fa := range e.plan.Deletes {
		if fa.Action != ActDownloadDelete {
			continue
		}

		if err := rb.Backup(fa.Path); err != nil {
			return err
		}
	}

	return nil
}

// removeLocalDeletedFiles removes local copies of REMOTE_DELETED files.
// Missing-file errors are tolerated for idempotency under retry.
func removeLocalDeletedFiles(e *Engine) error {
	for _, fa := range e.plan.Deletes {
		if fa.Action != ActDownloadDelete {
			continue
		}

		// phase5Execute already rejected unsafe server paths up front
		// via validateServerPaths; re-check here so the per-call-site
		// invariant survives future refactors that might bypass the
		// phase entry point.
		if err := fileops.SafeRelPath(fa.Path); err != nil {
			return fmt.Errorf("server returned unsafe delete path %q: %w", fa.Path, err)
		}

		abs := filepath.Join(e.projectDir, filepath.FromSlash(fa.Path))
		if err := os.Remove(abs); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", fa.Path, err)
		}
	}

	return nil
}

// validateServerPaths rejects plan entries whose Path is server-controlled
// and not safe to filepath.Join with e.projectDir. The local walker
// already validates Uploads and ActUploadDelete entries, so those are
// skipped to avoid over-rejecting benign locally-scanned paths.
func validateServerPaths(plan *SyncPlan) error {
	for _, fa := range plan.Downloads {
		if err := fileops.SafeRelPath(fa.Path); err != nil {
			return fmt.Errorf("server returned unsafe download path %q: %w", fa.Path, err)
		}
	}

	for _, fa := range plan.Conflicts {
		if err := fileops.SafeRelPath(fa.Path); err != nil {
			return fmt.Errorf("server returned unsafe conflict path %q: %w", fa.Path, err)
		}
	}

	for _, fa := range plan.Deletes {
		if fa.Action != ActDownloadDelete {
			continue
		}

		if err := fileops.SafeRelPath(fa.Path); err != nil {
			return fmt.Errorf("server returned unsafe delete path %q: %w", fa.Path, err)
		}
	}

	return nil
}

// pullList collects FileActions that need remote bytes pulled to disk.
// DEL_EDIT_CONFLICT is skipped because the remote deleted that file;
// only the local rename to .LOCAL.<ts> needs to happen.
func pullList(plan *SyncPlan) []FileAction {
	out := make([]FileAction, 0, len(plan.Downloads)+len(plan.Conflicts))
	out = append(out, plan.Downloads...)

	for _, fa := range plan.Conflicts {
		if fa.Classification == ClsDelEditConflict {
			continue
		}

		out = append(out, fa)
	}

	return out
}

// applyLocalDeletes is a no-op; applyDownloads already removed
// REMOTE_DELETED files.
func applyLocalDeletes(_ *Engine, _ *Rollback) error {
	return nil
}

// applyRemoteDeletesAndUploads sends LOCAL_DELETED paths to FilesAPI,
// runs the chosen Uploader for LOCAL_MODIFIED + LOCAL_ADDED, and PATCHes
// the artifact's codeRef so the workload picks up the new version.
func applyRemoteDeletesAndUploads(e *Engine, codeRef codeRefRef) (string, string, error) {
	catalogID := codeRef.CatalogID
	newCatalogID := catalogID
	newVersionID := codeRef.CatalogVersionID

	if vid, err := applyDeletes(e, catalogID); err != nil {
		return "", "", err
	} else if vid != "" {
		newVersionID = vid
	}

	if len(e.plan.Uploads) > 0 {
		uploader := ChooseUploader(e.plan)

		outcome, err := uploader.ApplyUploads(e, e.plan.Uploads)
		if err != nil {
			return "", "", err
		}

		// Store the outcome on the engine so Phase 6 can read
		// outcome.Sent[path]. The phase pipeline runs each phase
		// independently via runPhases with no per-phase return
		// threading, so Engine state is the only channel between
		// Phase 5 and Phase 6.
		e.uploadOutcome = &outcome

		newCatalogID = outcome.CatalogID
		newVersionID = outcome.VersionID
	}

	if newVersionID != "" && newVersionID != codeRef.CatalogVersionID {
		if err := e.artifacts.PatchCodeRef(e.config.ArtifactID, newCatalogID, newVersionID); err != nil {
			return "", "", fmt.Errorf("update artifact codeRef: %w", err)
		}
	}

	return newCatalogID, newVersionID, nil
}

// applyDeletes runs the LOCAL_DELETED to remote-delete step. Returns the
// new catalog version ID, or "" when nothing to delete or no catalog yet.
func applyDeletes(e *Engine, catalogID string) (string, error) {
	deletePaths := make([]string, 0)

	for _, fa := range e.plan.Deletes {
		if fa.Action == ActUploadDelete {
			deletePaths = append(deletePaths, fa.Path)
		}
	}

	if catalogID == "" || len(deletePaths) == 0 {
		return "", nil
	}

	resp, err := e.files.DeleteFiles(catalogID, deletePaths)
	if err != nil {
		return "", fmt.Errorf("delete remote files: %w", err)
	}

	if resp == nil {
		return "", nil
	}

	return resp.CatalogVersionID, nil
}

// verifyPostApplyUploads is --verify's second effect: after ApplyUploads,
// fetch the new version's file listing and confirm the server's checksum for
// every UPLOADED path equals the hash of the bytes this run streamed. The
// comparison is plain strings because the server's file checksum is the
// SHA-256 hex of the content, the same digest the uploader computed while
// streaming.
//
// It must run here in Phase 5, not in Phase 6: a mismatch has to fail the
// phase and trip the rollback while Phase 6 has written nothing — a Phase-6
// check would run after SaveManifest/SaveConfig, and failing there cannot
// un-write either file. The check is placed after the codeRef PATCH: the
// remote version cannot be un-published, so the recoverable posture after a
// failed verification is the drifted one — config still names the old
// version, the next sync sees the difference, fetches the real remote, and
// reconciles.
//
// Only uploaded paths are compared. Stage REPLACE merges staged paths into
// the version in place, so the listing legitimately contains files this sync
// never touched, some carrying checksums from earlier runs; flagging one of
// those would fail honest runs.
//
// numFiles from ApplyStage counts every file in the resulting version, not
// the ones uploaded, so it has no arithmetic relationship to the plan and is
// never used to gate, skip, or abort this check.
//
// The check is skipped entirely when nothing was uploaded: a downloads-only
// or empty plan makes no AllFiles request here, and a nil outcome or an
// empty Sent map is not an error.
func verifyPostApplyUploads(e *Engine, outcome *UploadOutcome) error {
	if !e.opts.Verify || outcome == nil || len(outcome.Sent) == 0 {
		return nil
	}

	listing, err := e.files.AllFiles(outcome.CatalogID, outcome.VersionID)
	if err != nil {
		return fmt.Errorf("post-apply verification: fetch files of version %s: %w", outcome.VersionID, err)
	}

	// Sorted so the reported path is deterministic when several differ.
	paths := make([]string, 0, len(outcome.Sent))

	for path := range outcome.Sent {
		paths = append(paths, path)
	}

	sort.Strings(paths)

	for _, path := range paths {
		sent := outcome.Sent[path]

		held, ok := listing[path]
		if !ok {
			return fmt.Errorf("post-apply verification: uploaded file %s is absent from server version %s", path, outcome.VersionID)
		}

		if held.Hash != sent.Hash {
			return fmt.Errorf("post-apply verification: server checksum for %s in version %s does not match the uploaded bytes (sent %s, server holds %s)", path, outcome.VersionID, sent.Hash, held.Hash)
		}
	}

	return nil
}
