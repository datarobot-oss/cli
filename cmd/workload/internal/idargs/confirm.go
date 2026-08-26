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

package idargs

import (
	"errors"
	"fmt"

	"github.com/datarobot/cli/cmd/helpers"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/spf13/cobra"
)

// AddYesFlag registers --yes for the commands that ask before acting.
//
// Only the environment variable is bound to viper: an explicit --yes read
// through viper would land in AllSettings() and persist to drconfig.yaml.
func AddYesFlag(cmd *cobra.Command, usage string) {
	cmd.Flags().BoolP("yes", "y", false, usage)

	_ = viperx.BindEnv("yes", reader.NonInteractiveEnv)
}

// Confirm puts the question, unless --yes or the non-interactive environment
// variable already answered it. A declined prompt is (false, nil) so the
// caller can exit 0 as a no-op; no terminal is an error, because there is
// nobody to ask.
//
// Whether to ask at all stays with the caller: `delete` always asks, while
// `stop` and `start` ask only when the workload was named by a manifest rather
// than by the user.
func Confirm(cmd *cobra.Command, question string) (bool, error) {
	yesFlag, _ := cmd.Flags().GetBool("yes")
	if yesFlag || viperx.GetBool("yes") {
		return true, nil
	}

	if !reader.IsStdinTerminal() {
		return false, errors.New(
			"confirmation required: pass --yes (or set " + reader.NonInteractiveEnv +
				"=1) to run without a prompt")
	}

	return helpers.Confirm(cmd.OutOrStdout(), cmd.InOrStdin(), question)
}

// AmbientPrompt is the question a mutating command asks about a workload the
// user did not name. It gives the manifest as well as the id, because an id
// the reader has never seen is nothing to compare against.
func AmbientPrompt(verb string, ref Ref) string {
	return fmt.Sprintf("%s workload %s, named by %s? [y/N] ", verb, ref.ID, ref.Path)
}
