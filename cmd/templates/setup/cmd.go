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

package setup

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "setup",
	Short: "🎉 Interactive template setup wizard",
	Long: `Launch the interactive template setup wizard to get started with DataRobot AI applications.

🎯 This wizard will help you:
  1️⃣  Choose an AI application template
  2️⃣  Clone it to your computer
  3️⃣  Configure your environment
  4️⃣  Get you ready to build!

⏱️ Takes about 3-5 minutes
🎉 You'll have a working AI app at the end

💡 Perfect for first-time users or someone starting a new project.`,
	PreRunE: auth.EnsureAuthenticatedE,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return RunTea(cmd.Context(), false)
	},
}

func init() {
	telemetry.Track(Cmd)
}

// RunTea starts the template setup TUI, optionally from the start command
func RunTea(ctx context.Context, fromStartCommand bool) error {
	m := NewModel(fromStartCommand)

	_, err := tui.Run(m, tea.WithAltScreen(), tea.WithContext(ctx))
	// TODO: Re-enable after further testing of component configure
	// if err != nil {
	// 	return err
	// }

	// // Check if we need to launch template setup after quitting
	// if setupModel, ok := finalModel.(tui.InterruptibleModel); ok {
	// 	if innerModel, ok := setupModel.Model.(Model); ok {
	// 		if innerModel.dotenvSetupCompleted {
	// 			return component.RunE(component.AddCmd, nil)
	// 		}
	// 	}
	// }

	return err
}

func InnerModel(finalModel tea.Model) (Model, bool) {
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
