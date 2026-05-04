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
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/datarobot/cli/internal/workload/sync"
)

// PrintPlan writes the human-readable sync plan to w. Empty plans print
// "Up to date." and return.
func PrintPlan(w io.Writer, plan *sync.SyncPlan) error {
	if plan == nil || plan.IsEmpty() {
		_, err := fmt.Fprintln(w, "Up to date.")
		return err
	}

	_, _ = fmt.Fprintln(w, "Sync plan:")

	if err := printGroup(w, "↑ UPLOAD", plan.Uploads, markerForUpload); err != nil {
		return err
	}

	if err := printGroup(w, "↓ DOWNLOAD", plan.Downloads, markerForDownload); err != nil {
		return err
	}

	if err := printGroup(w, "✕ DELETE", plan.Deletes, markerForDelete); err != nil {
		return err
	}

	if err := printGroup(w, "⚠ CONFLICT (remote wins)", plan.Conflicts, markerForConflict); err != nil {
		return err
	}

	return nil
}

func printGroup(w io.Writer, header string, files []sync.FileAction, marker func(sync.FileAction) string) error {
	if len(files) == 0 {
		return nil
	}

	if _, err := fmt.Fprintf(w, "  %s (%d):\n", header, len(files)); err != nil {
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	for _, fa := range files {
		size := formatSize(displaySize(fa))

		if _, err := fmt.Fprintf(tw, "    %s\t%s\t%s\n", marker(fa), fa.Path, size); err != nil {
			return err
		}
	}

	return tw.Flush()
}

func displaySize(fa sync.FileAction) int64 {
	switch fa.Action {
	case sync.ActUploadModify, sync.ActUploadAdd, sync.ActUploadDelete:
		return fa.LocalSize
	case sync.ActDownloadModify, sync.ActDownloadAdd, sync.ActDownloadDelete, sync.ActDownloadOverDel, sync.ActConflictCopy:
		return fa.RemoteSize
	case sync.ActSkip:
		return 0
	}

	return 0
}

func markerForUpload(fa sync.FileAction) string {
	if fa.Classification == sync.ClsLocalAdded {
		return "A"
	}

	return "M"
}

func markerForDownload(fa sync.FileAction) string {
	switch fa.Classification {
	case sync.ClsRemoteAdded:
		return "A"
	case sync.ClsRemoteDeleted:
		return "D"
	case sync.ClsUnchanged, sync.ClsLocalModified, sync.ClsRemoteModified, sync.ClsConverged,
		sync.ClsConflict, sync.ClsLocalAdded, sync.ClsAddConflict, sync.ClsLocalDeleted,
		sync.ClsBothDeleted, sync.ClsDelEditConflict, sync.ClsEditDelConflict, sync.ClsBothAddedSame:
		return "M"
	}

	return "M"
}

func markerForDelete(_ sync.FileAction) string {
	return "D"
}

func markerForConflict(_ sync.FileAction) string {
	return "⚠"
}

func formatSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KiB", float64(n)/1024.0)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MiB", float64(n)/1024.0/1024.0)
	}

	return fmt.Sprintf("%.1f GiB", float64(n)/1024.0/1024.0/1024.0)
}
