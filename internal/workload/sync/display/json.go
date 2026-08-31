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

package display

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/datarobot/cli/internal/workload/sync"
)

// PlanJSON is the JSON shape emitted for --output-format=json with
// --dry-run or --diff.
type PlanJSON struct {
	Uploads   []FileActionJSON `json:"uploads"`
	Downloads []FileActionJSON `json:"downloads"`
	Deletes   []FileActionJSON `json:"deletes"`
	Conflicts []FileActionJSON `json:"conflicts"`
	Stats     PlanStatsJSON    `json:"stats"`

	// Locked marks a plan that can never be applied, because the artifact it
	// was computed against is immutable. A preview of a locked artifact is the
	// one case where a full upload list means the opposite of what it looks
	// like, and a script reading only this document has nothing else to go on:
	// the human warning goes to stderr and the exit status is 0.
	Locked bool `json:"locked"`

	// Divergence lists the paths where manifest.json (BASE) disagrees with
	// the server (REMOTE), as detected by a --verify run. Like Locked it is
	// always emitted and explicitly empty when there is nothing to report, so
	// a script never has to guess whether a missing key meant "checked and
	// clean" or "never checked". Only a --verify run can populate it; every
	// other run leaves it empty. The findings are diagnostics: the plan
	// already reconciles them and the exit status is unchanged.
	Divergence []DivergenceJSON `json:"divergence"`

	// SkippedSymlinks lists every symlink the walk did not follow, filtered
	// through the ignore matcher. Like Locked and Divergence it is always
	// emitted and explicitly empty when there are none, so a script never
	// has to guess whether a missing key meant "no symlinks" or "never
	// checked". The findings are diagnostics: the plan already excludes them
	// and the exit status is unchanged.
	SkippedSymlinks []SkippedSymlinkJSON `json:"skippedSymlinks"`
}

// SkippedSymlinkJSON is one symlink the walk did not follow, in the plan
// document. IsDir distinguishes a single skipped file from an entire omitted
// subtree (a directory symlink prunes all of its children), which is the
// distinction that makes the notice actionable.
type SkippedSymlinkJSON struct {
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

// DivergenceJSON is one BASE-vs-REMOTE disagreement in the plan document.
// Kind is the fixed vocabulary shared with the stderr prose (hash_mismatch /
// base_only / remote_only); the hash of the side a kind does not involve is
// omitted, so a script can identify each finding without re-fetching.
type DivergenceJSON struct {
	Path       string              `json:"path"`
	Kind       sync.DivergenceKind `json:"kind"`
	BaseHash   string              `json:"baseHash,omitempty"`
	RemoteHash string              `json:"remoteHash,omitempty"`
}

type FileActionJSON struct {
	Path           string `json:"path"`
	Classification string `json:"classification"`
	LocalSize      int64  `json:"localSize,omitempty"`
	RemoteSize     int64  `json:"remoteSize,omitempty"`
	LocalHash      string `json:"localHash,omitempty"`
	RemoteHash     string `json:"remoteHash,omitempty"`
}

type PlanStatsJSON struct {
	UploadCount     int    `json:"uploadCount"`
	DownloadCount   int    `json:"downloadCount"`
	DeleteCount     int    `json:"deleteCount"`
	ConflictCount   int    `json:"conflictCount"`
	UploadBytes     int64  `json:"uploadBytes"`
	DownloadBytes   int64  `json:"downloadBytes"`
	OldVersionShort string `json:"oldVersionShort,omitempty"`
}

// RenderPlanJSON writes plan as pretty-printed JSON to w. divergences are
// the BASE-vs-REMOTE findings of a --verify run; skippedSymlinks are the
// symlinks the walk did not follow. Both are rendered even when plan is nil,
// because they are detected in Phase 2 regardless of what the plan goes on
// to do with the paths.
func RenderPlanJSON(w io.Writer, plan *sync.SyncPlan, locked bool, divergences []sync.Divergence, skippedSymlinks []sync.SkippedSymlink) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	if plan == nil {
		return enc.Encode(PlanJSON{
			Locked:          locked,
			Divergence:      divergencesJSON(divergences),
			SkippedSymlinks: skippedSymlinksJSON(skippedSymlinks),
		})
	}

	out := PlanJSON{
		Locked:          locked,
		Divergence:      divergencesJSON(divergences),
		SkippedSymlinks: skippedSymlinksJSON(skippedSymlinks),
		Uploads:         actionsJSON(plan.Uploads),
		Downloads:       actionsJSON(plan.Downloads),
		Deletes:         actionsJSON(plan.Deletes),
		Conflicts:       actionsJSON(plan.Conflicts),
		Stats: PlanStatsJSON{
			UploadCount:     len(plan.Uploads),
			DownloadCount:   len(plan.Downloads),
			DeleteCount:     len(plan.Deletes),
			ConflictCount:   len(plan.Conflicts),
			UploadBytes:     plan.TotalUploadBytes(),
			DownloadBytes:   plan.TotalDownloadBytes(),
			OldVersionShort: plan.OldVersionShort,
		},
	}

	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encode plan json: %w", err)
	}

	return nil
}

// divergencesJSON converts the engine's findings to the wire shape. The
// result is never nil: an empty input yields an explicit empty slice so the
// rendered document says "no divergence" instead of null.
func divergencesJSON(in []sync.Divergence) []DivergenceJSON {
	out := make([]DivergenceJSON, len(in))

	for i, d := range in {
		out[i] = DivergenceJSON{
			Path:       d.Path,
			Kind:       d.Kind,
			BaseHash:   d.BaseHash,
			RemoteHash: d.RemoteHash,
		}
	}

	return out
}

// skippedSymlinksJSON converts the engine's skipped-symlink findings to the
// wire shape. The result is never nil: an empty input yields an explicit
// empty slice so the rendered document says "no skipped symlinks" instead
// of null, mirroring the divergence and locked precedents.
func skippedSymlinksJSON(in []sync.SkippedSymlink) []SkippedSymlinkJSON {
	out := make([]SkippedSymlinkJSON, len(in))

	for i, s := range in {
		out[i] = SkippedSymlinkJSON{
			Path:  s.Path,
			IsDir: s.IsDir,
		}
	}

	return out
}

type ResultJSON struct {
	OldVersion      string   `json:"oldVersion,omitempty"`
	NewVersion      string   `json:"newVersion,omitempty"`
	UploadedCount   int      `json:"uploadedCount"`
	DownloadedCount int      `json:"downloadedCount"`
	DeletedCount    int      `json:"deletedCount"`
	ConflictCount   int      `json:"conflictCount"`
	ConflictCopies  []string `json:"conflictCopies,omitempty"`
	DurationMS      int64    `json:"durationMs"`
}

// RenderResultJSON writes result as pretty-printed JSON to w.
func RenderResultJSON(w io.Writer, r *sync.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	if r == nil {
		return enc.Encode(ResultJSON{})
	}

	out := ResultJSON{
		OldVersion:      r.OldVersion,
		NewVersion:      r.NewVersion,
		UploadedCount:   r.UploadedCount,
		DownloadedCount: r.DownloadedCount,
		DeletedCount:    r.DeletedCount,
		ConflictCount:   r.ConflictCount,
		ConflictCopies:  r.ConflictCopies,
		DurationMS:      r.Duration.Milliseconds(),
	}

	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encode result json: %w", err)
	}

	return nil
}

func actionsJSON(in []sync.FileAction) []FileActionJSON {
	out := make([]FileActionJSON, len(in))

	for i, fa := range in {
		out[i] = FileActionJSON{
			Path:           fa.Path,
			Classification: fa.Classification.String(),
			LocalSize:      fa.LocalSize,
			RemoteSize:     fa.RemoteSize,
			LocalHash:      fa.LocalHash,
			RemoteHash:     fa.RemoteHash,
		}
	}

	return out
}
