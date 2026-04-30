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
	"github.com/datarobot/cli/internal/dependencies"
	"github.com/datarobot/cli/internal/tools"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type Options struct {
	Yes bool
}

var opts Options

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "📦 Install missing template dependencies",
		RunE:  RunE,
	}
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, `Assume "yes" as answer to the install prompt.`)

	// Bind flag to viper to enable env var support (DATAROBOT_CLI_NON_INTERACTIVE)
	_ = viper.BindPFlag("yes", cmd.Flags().Lookup("yes"))
	_ = viper.BindEnv("yes", "DATAROBOT_CLI_NON_INTERACTIVE")

	return cmd
}

func RunE(cmd *cobra.Command, _ []string) error {
	missingTools, wrongVersionTools, missingMsgs, wrongVersionMsgs := tools.CheckPrerequisites()
	if len(missingMsgs) == 0 && len(wrongVersionMsgs) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "✅ All dependencies are already up to date.")
		return nil
	}

	fmt.Fprintln(cmd.OutOrStderr(), tools.PrerequisitesMsg(missingMsgs, wrongVersionMsgs))

	prerequisites := append(missingTools, wrongVersionTools...)

	if !viper.GetBool("yes") {
		yes, err := helpers.Confirm(cmd.OutOrStdout(), cmd.InOrStdin(), "\nInstall now? (y/n): ")
		if err != nil {
			cmd.SilenceUsage = true
			return err
		}

		if !yes {
			return nil
		}
	}

	err := dependencies.InstallPrerequisites(cmd.OutOrStdout(), prerequisites)
	if err != nil {
		cmd.SilenceUsage = true
		return err
	}

	return nil
}
