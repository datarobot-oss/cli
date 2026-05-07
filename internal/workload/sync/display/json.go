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

// RenderPlanJSON writes plan as pretty-printed JSON to w.
func RenderPlanJSON(w io.Writer, plan *sync.SyncPlan) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	if plan == nil {
		return enc.Encode(PlanJSON{})
	}

	out := PlanJSON{
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

	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encode plan json: %w", err)
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
