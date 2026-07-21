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

package selectcmd

import (
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "select [llm-id]",
		Short:        "Set the default LLM",
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		PreRunE:      auth.EnsureAuthenticatedE,
		RunE: func(cmd *cobra.Command, args []string) error {
			llmList, err := drapi.GetLLMsAndDeployed()
			if err != nil {
				return err
			}

			var chosenID string

			if len(args) == 1 {
				chosenID, err = findByID(llmList.LLMs, args[0])
			} else {
				chosenID, err = runPicker(llmList.LLMs)
			}

			if err != nil {
				return err
			}

			viperx.Set(config.DefaultLLMID, chosenID)

			if err = config.UpdateConfigFile(config.DefaultLLMID); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Default LLM set to: %s\n", chosenID)

			return nil
		},
	}

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"direct": len(args) == 1,
		}
	})

	return cmd
}

func findByID(llms []drapi.LLM, id string) (string, error) {
	for _, l := range llms {
		if l.LlmID == id {
			return l.LlmID, nil
		}
	}

	return "", fmt.Errorf("LLM %q not found", id)
}

func runPicker(llms []drapi.LLM) (string, error) {
	if len(llms) == 0 {
		return "", errors.New("no active LLMs available")
	}

	m := NewPickerModel(llms)

	finalModel, err := tui.Run(m, tea.WithAltScreen())
	if err != nil {
		return "", err
	}

	picker, ok := tui.Unwrap(finalModel).(PickerModel)
	if !ok {
		return "", errors.New("unexpected inner model type returned from TUI")
	}

	if picker.selectedID == "" {
		return "", errors.New("no LLM selected")
	}

	return picker.selectedID, nil
}
