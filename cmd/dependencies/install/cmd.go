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

package install

import (
	"fmt"

	"github.com/datarobot/cli/cmd/helpers"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/dependencies"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/tools"
	"github.com/spf13/cobra"
)

type Options struct {
	Yes bool
}

var opts Options

func Cmd() *cobra.Command {
	var (
		checkResult    tools.CheckResult
		installSuccess []string
		installError   string
		yesFlag        bool
		nonInteractive bool
	)

	cmd := &cobra.Command{
		Use:   "install",
		Short: "📦 Install missing template dependencies",
		RunE: func(cmd *cobra.Command, _ []string) error {
			yesFlag = opts.Yes
			nonInteractive = viperx.GetBool("yes")

			log.Debug("deps: install start", "yes", yesFlag, "non_interactive", nonInteractive)

			checkResult = tools.CheckPrerequisites()

			log.Debug("deps: install check result", "missing", len(checkResult.MissingMsgs), "wrong_version", len(checkResult.WrongVersionMsgs))

			if len(checkResult.MissingMsgs) == 0 && len(checkResult.WrongVersionMsgs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "✅ All dependencies are already up to date.")

				return nil
			}

			fmt.Fprintln(cmd.OutOrStderr(), tools.PrerequisitesMsg(checkResult))

			prerequisites := append(checkResult.MissingTools, checkResult.WrongVersionTools...)

			if !yesFlag && !nonInteractive {
				yes, err := helpers.Confirm(cmd.OutOrStdout(), cmd.InOrStdin(), "\nInstall now? (y/n): ")
				if err != nil {
					cmd.SilenceUsage = true

					return err
				}

				if !yes {
					log.Debug("deps: install declined by user")

					return nil
				}
			}

			log.Debug("deps: proceeding with install", "count", len(prerequisites))

			var err error

			installSuccess, err = dependencies.InstallPrerequisites(cmd.OutOrStdout(), prerequisites)
			if err != nil {
				installError = err.Error()
				cmd.SilenceUsage = true

				return err
			}

			log.Debug("deps: install complete", "installed", installSuccess)

			return nil
		},
	}

	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, `Assume "yes" as answer to the install prompt.`)

	_ = viperx.BindEnv("yes", "DATAROBOT_CLI_NON_INTERACTIVE")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"missing_deps":          checkResult.MissingMsgs,
			"wrong_version_deps":    checkResult.WrongVersionMsgs,
			"validation_violations": checkResult.ValidationViolations,
			"install_success":       installSuccess,
			"install_error":         installError,
			"yes_flag":              yesFlag,
			"non_interactive":       nonInteractive,
		}
	})

	return cmd
}
