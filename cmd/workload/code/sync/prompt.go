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

package syncc

import (
	"fmt"
	"os"
	"strings"

	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/datarobot/cli/internal/workload/sync/display"
	"github.com/spf13/cobra"
)

// promptChoice is the symbolic return type of promptConflictMenu so
// callers don't string-compare in two places.
type promptChoice int

const (
	promptSync  promptChoice = iota // user accepted the plan; proceed to Execute
	promptDiffs                     // user wants per-file diffs first
	promptQuit                      // user aborted
)

// promptReadLine is the stdin seam. Tests reassign it to drive the
// conflict menu deterministically without hijacking os.Stdin.
var promptReadLine = reader.ReadString

// promptConflictMenu shows the [d] [Enter] [q] menu when conflicts
// exist and the user has not passed --yes. Loops on [d] so the user
// can review diffs and then either confirm or abort. Prompts go to
// stderr to keep stdout clean for piped output.
func promptConflictMenu(cmd *cobra.Command, engine engineRunner, plan *sync.SyncPlan) (promptChoice, error) {
	for {
		fmt.Fprint(os.Stderr, "  [d] Show diffs  [Enter] Sync  [q] Abort: ")

		raw, err := promptReadLine()
		if err != nil {
			return promptQuit, err
		}

		switch strings.TrimSpace(strings.ToLower(raw)) {
		case "":
			return promptSync, nil
		case "d":
			if err := display.PrintDiffs(cmd.OutOrStdout(), plan, engine.Fetcher()); err != nil {
				return promptQuit, err
			}
			// loop and re-prompt
		case "q":
			return promptQuit, nil
		default:
			fmt.Fprintln(os.Stderr, "  (please type 'd', 'q', or press Enter)")
		}
	}
}
