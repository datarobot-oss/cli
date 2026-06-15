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

package checkout

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"

	"github.com/datarobot/cli/cmd/artifact/code/internal/format"
	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/workload"
)

type downloadResult struct {
	VersionID   string         `json:"versionId"`
	CheckoutDir string         `json:"checkoutDir"`
	FileCount   int            `json:"fileCount"`
	TotalSize   int64          `json:"totalSize"`
	Files       []downloadFile `json:"files"`
}

type downloadFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Hash string `json:"hash"`
}

type cleanResult struct {
	Removed []string `json:"removed"`
}

// checkoutDir is rendered relative to projectDir so output is portable.
func downloadView(projectDir, versionID, checkoutDir string, files map[string]filesapi.FileMeta) downloadResult {
	rows := make([]downloadFile, 0, len(files))

	var total int64

	for path, meta := range files {
		rows = append(rows, downloadFile{Path: path, Size: meta.Size, Hash: meta.Hash})

		total += meta.Size
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Path < rows[j].Path })

	rel, err := filepath.Rel(projectDir, checkoutDir)
	if err != nil {
		rel = checkoutDir
	}

	return downloadResult{
		VersionID:   versionID,
		CheckoutDir: rel,
		FileCount:   len(files),
		TotalSize:   total,
		Files:       rows,
	}
}

func renderDownloadResult(out io.Writer, outFmt workload.OutputFormat, r downloadResult) error {
	if outFmt == workload.OutputFormatJSON {
		return encodeJSON(out, r)
	}

	fmt.Fprintf(out, "Downloading version %s (%d files, %s)...\n", r.VersionID, r.FileCount, format.Bytes(r.TotalSize))

	for _, f := range r.Files {
		fmt.Fprintf(out, "  ↓ %s (%s)\n", f.Path, format.Bytes(f.Size))
	}

	fmt.Fprintf(out, "Checked out to: %s\n\n", r.CheckoutDir)
	fmt.Fprintln(out, "This is a read-only snapshot. Your working directory and sync state")
	fmt.Fprintln(out, "are untouched.")

	return nil
}

func renderCleanResult(out io.Writer, outFmt workload.OutputFormat, r cleanResult) error {
	if outFmt == workload.OutputFormatJSON {
		return encodeJSON(out, r)
	}

	switch len(r.Removed) {
	case 0:
		fmt.Fprintln(out, "No checkouts to remove.")
	case 1:
		fmt.Fprintf(out, "Removed checkout %s\n", r.Removed[0])
	default:
		fmt.Fprintf(out, "Removed %d checkouts:\n", len(r.Removed))

		for _, name := range r.Removed {
			fmt.Fprintf(out, "  - %s\n", name)
		}
	}

	return nil
}

func encodeJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")

	return enc.Encode(v)
}
