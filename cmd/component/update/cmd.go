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

package update

import (
	"errors"
	"fmt"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/cmd/component/shared"
	"github.com/datarobot/cli/cmd/task/compose"
	"github.com/datarobot/cli/internal/appframework"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/tools"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

type updateFlags struct {
	DataArgs []string
	DataFile string
	Filter   []string
}

var flags updateFlags

func PreRunE(_ *cobra.Command, _ []string) error {
	if !repo.IsInRepoRoot() {
		return errors.New("You must be in the repository root directory.")
	}

	if err := tools.CheckPrerequisite("uv"); err != nil {
		return err
	}

	return nil
}

func runPostUpdateTasks() error {
	if err := appframework.ExecRunTasks("."); err != nil {
		return err
	}

	return compose.Cmd().RunE(nil, nil)
}

func handleTUIResult(finalModel tea.Model) error {
	setupModel, ok := finalModel.(tui.InterruptibleModel)
	if !ok {
		return nil
	}

	innerModel, ok := setupModel.Model.(shared.UpdateModel)
	if !ok {
		return nil
	}

	fmt.Println(innerModel.ExitMessage)

	if !innerModel.ComponentUpdated {
		return nil
	}

	if err := runPostUpdateTasks(); err != nil {
		return err
	}

	fmt.Println(innerModel.ExitMessage)
	fmt.Println("Post-update tasks finished.")

	return nil
}

func RunE(_ *cobra.Command, args []string) error {
	var label string

	if len(args) > 0 {
		label = args[0]
	}

	cliData, err := shared.ParseDataArgs(flags.DataArgs)
	if err != nil {
		return fmt.Errorf("parsing data args: %w", err)
	}

	// If a label was provided directly, run the update non-interactively.
	if label != "" {
		if err := runUpdate(label, cliData, flags.DataFile); err != nil {
			return fmt.Errorf("updating component: %w", err)
		}

		return runPostUpdateTasks()
	}

	m := shared.NewUpdateComponentModel(flags.DataArgs, flags.DataFile)

	finalModel, err := tui.Run(m, tea.WithAltScreen())
	if err != nil {
		return err
	}

	return handleTUIResult(finalModel)
}

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "update [label]",
		Short:         "🔄 Update installed component",
		PreRunE:       PreRunE,
		RunE:          RunE,
		SilenceErrors: true,
	}

	cmd.Flags().StringArrayVarP(&flags.DataArgs, "data", "d", []string{}, "Provide answer data in key=value format (can be specified multiple times)")
	cmd.Flags().StringVar(&flags.DataFile, "data-file", "", "Path to YAML file with default answers")
	cmd.Flags().StringArrayVarP(&flags.Filter, "filter", "F", []string{}, "Restrict update to specific labels (can be specified multiple times)")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"component_name": telemetry.FirstArg(args),
		}
	})

	return cmd
}

func runUpdate(label string, cliData map[string]interface{}, dataFilePath string) error {
	fw := shared.GetFrameworkPath()

	// Load component defaults and merge with CLI data.
	componentConfig, err := config.LoadComponentDefaults(dataFilePath)
	if err != nil {
		log.Warn("Failed to load component defaults", "error", err)

		componentConfig = &config.ComponentDefaults{
			Defaults: make(map[string]map[string]interface{}),
		}
	}

	mergedData := componentConfig.MergeWithCLIData(label, cliData)

	// Pre-supply any known answers before the three-way merge update.
	if err := appframework.ExecAnswer(label, mergedData, fw, "."); err != nil {
		return fmt.Errorf("answering questions for %q: %w", label, err)
	}

	filter := append(flags.Filter, label) //nolint:gocritic

	execErr := appframework.ExecUpdate(filter, fw, ".")
	if execErr != nil {
		if errors.Is(execErr, exec.ErrNotFound) {
			log.Error("uv is not installed.")
		}

		return execErr
	}

	return nil
}
