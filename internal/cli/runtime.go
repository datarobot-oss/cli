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

// runtime.go provides helpers that commands call during execution to query the
// ambient runtime context — specifically whether the current invocation is
// running in a non-interactive (automated) mode.
//
// IsNonInteractive is the single authoritative check for prompt-skipping. Any
// new automation signal (a future --no-prompt flag, a CI env var, etc.) should
// be added here rather than inlined in individual commands.
package cli

import (
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/spf13/cobra"
)

// IsNonInteractive reports whether the current command invocation should skip prompts.
// It merges the non-interactive environment variable, any explicit --yes flag, and the
// global --force-interactive override into a single truthy signal for automation.
func IsNonInteractive(cmd *cobra.Command) bool {
	if viperx.GetBool("force-interactive") {
		return false
	}

	if reader.IsNonInteractive() {
		return true
	}

	if cmd == nil {
		return viperx.GetBool(YesFlagName)
	}

	flag := cmd.Flags().Lookup(YesFlagName)
	if flag != nil {
		if yes, err := cmd.Flags().GetBool(YesFlagName); err == nil && yes {
			return true
		}
	}

	return viperx.GetBool(YesFlagName)
}
