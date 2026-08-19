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

// Package telemetry provides Amplitude-based usage analytics for the DataRobot CLI.
// This file handles detection and stamping of the interaction mode (interactive vs
// non-interactive) so every event carries consistent metadata.
package telemetry

import (
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/spf13/cobra"
)

// yesFlagName is the canonical name of the per-command flag that opts the
// caller into skipping interactive prompts.
const yesFlagName = "yes"

// StampInteractionMode annotates props with whether the current invocation is
// running in non-interactive mode. This is called once in the root command's
// PersistentPreRunE so every downstream event shares the same metadata without
// per-command duplication.
func StampInteractionMode(props *CommonProperties, cmd *cobra.Command) {
	if props == nil {
		return
	}

	props.NonInteractive = computeNonInteractive(cmd)
}

// computeNonInteractive evaluates the global sources that flip the CLI into
// non-interactive behaviour: the force-interactive viper override, the
// DATAROBOT_CLI_NON_INTERACTIVE env var, and any per-command --yes flag.
func computeNonInteractive(cmd *cobra.Command) bool {
	// force-interactive takes precedence even when automation flags are present,
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

// hasYesFlag reports whether cmd (or its inherited flag set) was invoked with
// --yes. Returns false when cmd is nil, the flag is absent, or the flag is not
// a bool.
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
