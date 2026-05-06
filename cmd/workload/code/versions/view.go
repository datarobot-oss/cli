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

package versions

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/workload"
)

const shortVersionLen = 8

// view is the rendered shape, decoupled from the API types.
type view struct {
	ArtifactID       string
	ArtifactName     string
	ArtifactStatus   string
	Versions         []versionRow
	CurrentVersionID string
	SyncedVersionID  string
}

type versionRow struct {
	ID        string
	Short     string
	CreatedAt string
	NumFiles  int
	TotalSize int64
	IsCurrent bool
}

func newView(art workload.Artifact, vs []filesapi.CatalogVersion, currentID, syncedID string) view {
	rows := make([]versionRow, len(vs))
	for i, v := range vs {
		rows[i] = versionRow{
			ID:        v.ID,
			Short:     shortID(v.ID),
			CreatedAt: v.CreatedAt,
			NumFiles:  v.NumFiles,
			TotalSize: v.TotalSize,
			IsCurrent: v.ID == currentID,
		}
	}

	return view{
		ArtifactID:       art.ID,
		ArtifactName:     art.Name,
		ArtifactStatus:   strings.ToUpper(art.Status),
		Versions:         rows,
		CurrentVersionID: currentID,
		SyncedVersionID:  syncedID,
	}
}

func shortID(id string) string {
	if len(id) > shortVersionLen {
		return id[:shortVersionLen]
	}

	return id
}

// renderText prints the human-readable table.
func renderText(out io.Writer, v view) {
	fmt.Fprintf(out, "Artifact: %s (%s)\n", v.ArtifactName, v.ArtifactID)
	fmt.Fprintf(out, "Status:   %s\n\n", v.ArtifactStatus)

	if len(v.Versions) == 0 {
		fmt.Fprintln(out, "No versions found.")

		return
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "VERSION ID\tFILES\tSIZE\tCREATED AT")

	for _, row := range v.Versions {
		marker := "  "
		if row.IsCurrent {
			marker = "* "
		}

		fmt.Fprintf(
			w, "%s%s\t%d\t%s\t%s\n",
			marker,
			row.Short,
			row.NumFiles,
			humanBytes(row.TotalSize),
			formatCreatedAt(row.CreatedAt),
		)
	}

	w.Flush()

	if v.CurrentVersionID != "" {
		fmt.Fprintln(out, "\n* = current (artifact codeRef)")
	}

	if v.SyncedVersionID != "" {
		fmt.Fprintf(out, "Local synced to: %s\n", shortID(v.SyncedVersionID))
	}
}

// jsonRow is the output schema for --output-format json.
type jsonRow struct {
	VersionID    string `json:"versionId"`
	VersionShort string `json:"versionShort"`
	CreatedAt    string `json:"createdAt"`
	FileCount    int    `json:"fileCount"`
	TotalSize    int64  `json:"totalSize"`
	IsCurrent    bool   `json:"isCurrent"`
}

func renderJSON(out io.Writer, v view) error {
	rows := make([]jsonRow, len(v.Versions))
	for i, r := range v.Versions {
		rows[i] = jsonRow{
			VersionID:    r.ID,
			VersionShort: r.Short,
			CreatedAt:    r.CreatedAt,
			FileCount:    r.NumFiles,
			TotalSize:    r.TotalSize,
			IsCurrent:    r.IsCurrent,
		}
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")

	return enc.Encode(rows)
}

func formatCreatedAt(s string) string {
	if s == "" {
		return "—"
	}

	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return s
		}
	}

	return t.UTC().Format("2006-01-02 15:04 UTC")
}

// humanBytes renders an int64 byte count as KB/MB/GB with one decimal.
// 1 KB = 1024 B (binary), matching how engine and limits already think.
func humanBytes(n int64) string {
	const unit = 1024

	if n < unit {
		return fmt.Sprintf("%d B", n)
	}

	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}

	suffix := []string{"KB", "MB", "GB", "TB", "PB"}[exp]

	return fmt.Sprintf("%.1f %s", float64(n)/float64(div), suffix)
}
