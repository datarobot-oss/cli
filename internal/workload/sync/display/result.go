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
	"strings"

	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/datarobot/cli/tui"
)

// PrintResult emits the one-line summary for a successful sync. Zero
// counts are omitted from the trailing parens.
func PrintResult(w io.Writer, r *sync.Result) error {
	if r == nil {
		return nil
	}

	old := sync.ShortVer(r.OldVersion)
	if old == "" {
		old = "∅"
	}

	newVer := sync.ShortVer(r.NewVersion)
	counts := formatCounts(r)

	line := fmt.Sprintf("Sync complete: %s → %s  %s", old, newVer, counts)
	styled := tui.SuccessStyle.Render(line)

	if _, err := fmt.Fprintln(w, styled); err != nil {
		return err
	}

	if len(r.ConflictCopies) > 0 {
		_, _ = fmt.Fprintln(w, tui.DimStyle.Render("Conflict copies saved:"))

		for _, p := range r.ConflictCopies {
			_, _ = fmt.Fprintln(w, tui.DimStyle.Render("  "+p))
		}
	}

	return nil
}

func formatCounts(r *sync.Result) string {
	parts := make([]string, 0, 4)

	if r.UploadedCount > 0 {
		parts = append(parts, fmt.Sprintf("↑%d", r.UploadedCount))
	}

	if r.DownloadedCount > 0 {
		parts = append(parts, fmt.Sprintf("↓%d", r.DownloadedCount))
	}

	if r.DeletedCount > 0 {
		parts = append(parts, fmt.Sprintf("✕%d", r.DeletedCount))
	}

	if r.ConflictCount > 0 {
		parts = append(parts, fmt.Sprintf("⚠%d", r.ConflictCount))
	}

	if len(parts) == 0 {
		return "(no changes)"
	}

	return "(" + strings.Join(parts, " ") + ")"
}
