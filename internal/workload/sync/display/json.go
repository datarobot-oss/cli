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

// SyncJSON is the single document `dr artifact code sync --output-format json`
// emits. The plan's fields sit at the top level (its shape unchanged), and the
// result of a version-writing run is nested under "result" when one ran.
//
// One document per invocation: emitting the plan and the result as two
// concatenated top-level objects made stdout unparseable by a plain json.loads
// and forced every consumer to write a splitter (RAPTOR-19348).
//
// The plan sits at the top level (result nested), which is the opposite of
// `dr workload up` (scalars at the top level, plan under "plan"). The shapes
// differ deliberately: this command's documented output is the plan's own
// uploads[]/downloads[]/conflicts[] keys, so they have to stay at the root that
// existing consumers read.
type SyncJSON struct {
	PlanJSON

	Result *ResultJSON `json:"result,omitempty"`

	// Refused is set when the plan was emitted but nothing was applied because
	// it needs a confirmation the run could not give (conflicts under
	// --output-format json without --yes). It is the positive signal that the
	// run is a no-op — uploads included — so a consumer does not have to infer
	// that from a missing "result" (RAPTOR-19348).
	Refused bool `json:"refused,omitempty"`
}

// planJSON builds the plan view. A nil plan carries only the locked flag, which
// is all a preview of an unreadable plan can report.
func planJSON(plan *sync.SyncPlan, locked bool) PlanJSON {
	if plan == nil {
		return PlanJSON{Locked: locked}
	}

	return PlanJSON{
		Locked:    locked,
		Uploads:   actionsJSON(plan.Uploads),
		Downloads: actionsJSON(plan.Downloads),
		Deletes:   actionsJSON(plan.Deletes),
		Conflicts: actionsJSON(plan.Conflicts),
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
}

// RenderSyncJSON writes the one-document sync view to w: the plan, plus the
// result under "result" when result is non-nil.
func RenderSyncJSON(w io.Writer, plan *sync.SyncPlan, result *sync.Result, locked bool) error {
	out := SyncJSON{PlanJSON: planJSON(plan, locked)}

	if result != nil {
		r := resultJSON(result)
		out.Result = &r
	}

	return encodeSyncJSON(w, out)
}

// RenderRefusedJSON writes the plan with "refused": true and no result: the run
// declined to apply anything because the plan needs a confirmation it could not
// give (conflicts, no --yes). One document, like the others.
func RenderRefusedJSON(w io.Writer, plan *sync.SyncPlan, locked bool) error {
	return encodeSyncJSON(w, SyncJSON{PlanJSON: planJSON(plan, locked), Refused: true})
}

func encodeSyncJSON(w io.Writer, out SyncJSON) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encode sync json: %w", err)
	}

	return nil
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

// resultJSON builds the result view of a completed sync.
func resultJSON(r *sync.Result) ResultJSON {
	return ResultJSON{
		OldVersion:      r.OldVersion,
		NewVersion:      r.NewVersion,
		UploadedCount:   r.UploadedCount,
		DownloadedCount: r.DownloadedCount,
		DeletedCount:    r.DeletedCount,
		ConflictCount:   r.ConflictCount,
		ConflictCopies:  r.ConflictCopies,
		DurationMS:      r.Duration.Milliseconds(),
	}
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
