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

// Package format provides display helpers shared across workload-code commands.
package format

import (
	"fmt"
	"io"

	"github.com/datarobot/cli/tui"
)

// StateNotice prints an incidental note about the state directory to w, dimmed
// like the other housekeeping chatter. An empty notice prints nothing, so a
// caller can pass wapi.LegacyStateNotice's result straight through. Send it to
// stderr: it is never part of a command's data output.
func StateNotice(w io.Writer, notice string) {
	if notice == "" {
		return
	}

	fmt.Fprintln(w, tui.DimStyle.Render(notice))
}

// Bytes renders n in base-1024 units, capping at PB to avoid suffix overflow.
func Bytes(n int64) string {
	const unit = 1024

	if n < unit {
		return fmt.Sprintf("%d B", n)
	}

	suffixes := []string{"KB", "MB", "GB", "TB", "PB"}
	div, exp := int64(unit), 0

	for x := n / unit; x >= unit && exp < len(suffixes)-1; x /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %s", float64(n)/float64(div), suffixes[exp])
}
