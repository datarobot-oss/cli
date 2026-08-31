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

	"github.com/datarobot/cli/internal/workload/fileops"
	"github.com/datarobot/cli/internal/workload/ignore"
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

	// Phase 6 discards the rollback only after SaveConfig + SaveManifest
	// succeed.
	e.rollback = rb

	return nil
}

func executePlan(e *Engine, rb *Rollback) error {
	// Every local file a remote-side action would replace or delete is moved to
	// a *.LOCAL.<ts> copy first, so nothing local is ever destroyed without a
	// recoverable backup. This subsumes the old REMOTE_DELETED os.Remove: those
	// files are renamed aside here, not removed.
	if err := backupOverwrittenLocals(e, rb); err != nil {
		return fmt.Errorf("back up overwritten local files: %w", err)
	}

	codeRef := codeRefOrEmpty(e)

	if err := applyDownloads(e, rb, codeRef); err != nil {
		return fmt.Errorf("downloads: %w", err)
	}

	newCatalogID, newVersionID, err := applyRemoteDeletesAndUploads(e, codeRef)
	if err != nil {
		return err
	}

	e.newCatalogID = newCatalogID
	e.newVersionID = newVersionID

	return nil
}

// backupStampFormat is the ISO-8601 basic UTC stamp appended after
// ignore.BackupInfix to name a *.LOCAL backup. ignore.IsBackupCopy matches this
// shape to keep backups out of the next sync; the two must stay in step.
const backupStampFormat = "20060102T150405Z"

// backupOverwrittenLocals moves every local file a remote-side action would
// replace or delete to <path>.LOCAL.<stamp>, before any of those actions run.
// It covers the three ways remote wins: a REMOTE_MODIFIED download (remote
// bytes are about to land over the local file), a REMOTE_DELETED delete (the
// local file is about to be removed), and a conflict (both sides changed).
// REMOTE_ADDED and EDIT_DEL have no local file to keep, so they are not here.
//
// The rename frees the original path: applyDownloads then writes the remote
// bytes there for downloads and conflicts, while REMOTE_DELETED has no download
// and so simply stays gone, which is what the remote deletion asked for.
func backupOverwrittenLocals(e *Engine, rb *Rollback) error {
	stamp := e.nowFn().UTC().Format(backupStampFormat)

	// OverwrittenLocalPaths is exactly the set of local files a remote-side
	// action would replace or delete, so backing up that set is the whole job.
	for _, path := range e.plan.OverwrittenLocalPaths() {
		if err := backupLocalFile(e, rb, path, stamp); err != nil {
			return err
		}
	}

	return nil
}

// backupLocalFile renames one local file to <path>.LOCAL.<stamp>, recording it
// for rollback and for the result. A missing or non-regular local file is a
// no-op: the remote-side action that follows still runs (a REMOTE_ADDED writes
// a new file; an already-absent file has nothing to keep).
func backupLocalFile(e *Engine, rb *Rollback, path, stamp string) error {
	// phase5Execute already rejected unsafe server paths up front via
	// validateServerPaths; re-check here so the per-call-site invariant
	// survives future refactors that might reach this without the phase entry
	// point, before Lstat/Rename touch anything outside projectDir.
	if err := fileops.SafeRelPath(path); err != nil {
		return fmt.Errorf("server returned unsafe path %q: %w", path, err)
	}

	src := filepath.Join(e.projectDir, filepath.FromSlash(path))

	info, err := os.Lstat(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("stat %s for backup: %w", path, err)
	}

	if !info.Mode().IsRegular() {
		return nil
	}

	dst := src + ignore.BackupInfix + stamp

	if err := rb.Backup(path); err != nil {
		return err
	}

	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", path, dst, err)
	}

	rb.TrackCreated(dst)
	e.localBackups = append(e.localBackups, dst)

	return nil
}

// applyDownloads pulls remote bytes for downloads and conflicts (remote wins,
// so remote bytes land at the original path, which backupOverwrittenLocals has
// already freed). REMOTE_DELETED needs no download: its local file was renamed
// aside and stays gone.
func applyDownloads(e *Engine, rb *Rollback, codeRef codeRefRef) error {
	if codeRef.CatalogID == "" || codeRef.CatalogVersionID == "" {
		return nil
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

		cid, vid, err := uploader.ApplyUploads(e, e.plan.Uploads)
		if err != nil {
			return "", "", err
		}

		newCatalogID = cid
		newVersionID = vid
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
