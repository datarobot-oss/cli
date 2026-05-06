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
)

// phase5Execute applies the SyncPlan in the order: disk-space check +
// rollback dir, conflict copies, downloads + local-side deletes, remote
// deletes, uploads. Any error after the rollback dir is created triggers
// a Restore to pre-sync state.
func phase5Execute(e *Engine) error {
	if e.plan == nil || e.plan.IsEmpty() {
		return nil
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

		abs := filepath.Join(e.projectDir, filepath.FromSlash(fa.Path))
		if err := os.Remove(abs); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", fa.Path, err)
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

		cid, vid, err := uploader.ApplyUploads(e, e.plan.Uploads)
		if err != nil {
			return "", "", err
		}

		newCatalogID = cid
		newVersionID = vid
	}

	if newVersionID != "" && newVersionID != codeRef.CatalogVersionID {
		if err := e.patchArtifactFn(e.config.ArtifactID, newCatalogID, newVersionID); err != nil {
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
