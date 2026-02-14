// Copyright 2025 DataRobot, Inc. and its affiliates.
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

package start

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/cmd/templates/setup"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

type Options struct {
	AnswerYes bool
}

func Cmd() *cobra.Command { //nolint: cyclop
	var opts Options

	cmd := &cobra.Command{
		Use:     "start",
		Aliases: []string{"quickstart"},
		GroupID: "core",
		Short:   "🚀 Run the application quickstart process",
		Long: `Run the application quickstart process for the current template.
The following actions will be performed:
- Checking for prerequisite tooling
- Executing the start script associated with the template, if available.`,
		PreRunE: auth.EnsureAuthenticatedE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m := NewStartModel(opts)

			finalModel, err := tui.Run(m)
			if err != nil {
				return err
			}

			innerModel, ok := getInnerModel(finalModel)
			if !ok {
				return nil
			}

			if innerModel.err != nil {
				os.Exit(1)
			}

			// Check if we do not need to launch template setup after quitting
			if !innerModel.needTemplateSetup || !innerModel.done || innerModel.quitting {
				return nil
			}

			// Need to run template setup
			// After it completes, we'll be in the cloned directory,
			// so we can just run start again
			sm := setup.NewModel(true)

			finalSetupModel, err := tui.Run(sm, tea.WithAltScreen(), tea.WithContext(cmd.Context()))
			if err != nil {
				return err
			}

			innerSetupModel, ok := setup.InnerModel(finalSetupModel)
			if ok && innerSetupModel.ExitMessage != "" {
				os.Exit(1)
			}

			// Now run start again - we're in the cloned repo directory
			// Create a new start model and run it
			m2 := NewStartModel(opts)

			finalModel2, err := tui.Run(m2)
			if err != nil {
				return err
			}

			innerModel2, ok := getInnerModel(finalModel2)
			if !ok {
				return nil
			}

			if innerModel2.err != nil {
				os.Exit(1)
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&opts.AnswerYes, "yes", "y", false, "Assume \"yes\" as answer to all prompts.")

	return cmd
}

func getInnerModel(finalModel tea.Model) (Model, bool) {
	startModel, ok := finalModel.(tui.InterruptibleModel)
	if !ok {
		return Model{}, false
	}

	innerModel, ok := startModel.Model.(Model)
	if !ok {
		return Model{}, false
	}

	return innerModel, true
}
