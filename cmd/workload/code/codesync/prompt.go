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

package codesync

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/datarobot/cli/internal/workload/sync/display"
	"github.com/spf13/cobra"
)

// promptChoice is the symbolic return type of promptConflictMenu so
// callers don't string-compare in two places.
type promptChoice int

const (
	promptSync promptChoice = iota // user accepted the plan; proceed to Execute
	promptQuit                     // user aborted
)

// promptConflictMenu shows the [d] [Enter] [q] menu when conflicts
// exist and the user has not passed --yes. Loops on [d] so the user
// can review diffs and then either confirm or abort. Prompts go to
// the cobra command's stderr (so cmd.SetErr in tests can capture them
// and embedded callers can redirect) to keep stdout clean for piped
// output. readLine is injected so tests can drive input deterministically.
func promptConflictMenu(cmd *cobra.Command, engine engineRunner, plan *sync.SyncPlan, readLine func() (string, error)) (promptChoice, error) {
	stderr := cmd.ErrOrStderr()

	for {
		fmt.Fprint(stderr, "  [d] Show diffs  [Enter] Sync  [q] Abort: ")

		raw, err := readLine()
		if err != nil {
			// EOF (Ctrl+D, closed pipe) is a clean abort: the user has
			// no way to confirm, so don't proceed. Other read errors
			// (terminal in a broken state, unexpected I/O) shouldn't
			// silently masquerade as a deliberate quit; log them at
			// debug so --debug surfaces what happened and still treat
			// the choice as quit so we never auto-apply on garbage.
			if !errors.Is(err, io.EOF) {
				log.Debug("conflict prompt read failed", "err", err)
			}

			return promptQuit, nil
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
			fmt.Fprintln(stderr, "  (please type 'd', 'q', or press Enter)")
		}
	}
}
