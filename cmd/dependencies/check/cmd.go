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

package check

import (
	"errors"
	"fmt"

	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/tools"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var result tools.CheckResult

	cmd := &cobra.Command{
		Use:   "check",
		Short: "✅ Check template dependencies",
		RunE: func(cmd *cobra.Command, _ []string) error {
			log.Debug("deps: check start")

			result = tools.CheckPrerequisites()

			log.Debug("deps: check result", "missing", len(result.MissingMsgs), "wrong_version", len(result.WrongVersionMsgs))

			if len(result.MissingMsgs) > 0 || len(result.WrongVersionMsgs) > 0 {
				cmd.SilenceUsage = true

				return errors.New(tools.PrerequisitesMsg(result.MissingMsgs, result.WrongVersionMsgs))
			}

			fmt.Fprintln(cmd.OutOrStdout(), "✅ All dependencies are already up to date.")

			return nil
		},
	}

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"missing_deps":          result.MissingMsgs,
			"wrong_version_deps":    result.WrongVersionMsgs,
			"validation_violations": result.ValidationViolations,
		}
	})

	return cmd
}
