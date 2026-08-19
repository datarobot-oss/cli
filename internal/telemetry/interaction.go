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

package telemetry

import (
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/spf13/cobra"
)

const yesFlagName = "yes"

// StampInteractionMode annotates props with whether the current invocation is running
// in non-interactive mode. This happens once in root PersistentPreRunE so every event
// shares the same interaction metadata without per-command duplication.
func StampInteractionMode(props *CommonProperties, cmd *cobra.Command) {
	if props == nil {
		return
	}

	props.NonInteractive = computeNonInteractive(cmd)
}

// computeNonInteractive evaluates the global sources that flip the CLI into
// non-interactive behavior: the env override, per-command --yes flag, and any
// global flag that forces interactivity.
func computeNonInteractive(cmd *cobra.Command) bool {
	// force-interactive takes precedence, even when automation flags are present,
	// because the caller explicitly asked to surface wizards.
	if viperx.GetBool("force-interactive") {
		return false
	}

	// DATAROBOT_CLI_NON_INTERACTIVE=true is the canonical signal for automation.
	if reader.IsNonInteractive() {
		return true
	}

	// --yes is the per-command opt-in to skip prompts.
	if hasYesFlag(cmd) {
		return true
	}

	return false
}

// hasYesFlag reports whether cmd (or its inherited flag set) was invoked with --yes.
func hasYesFlag(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}

	yesFlag := cmd.Flags().Lookup(yesFlagName)
	if yesFlag == nil {
		return false
	}

	yesValue, err := cmd.Flags().GetBool(yesFlagName)
	if err != nil {
		// A missing or non-bool flag should behave as "not set".
		return false
	}

	return yesValue
}
