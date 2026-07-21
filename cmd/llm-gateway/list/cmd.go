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

package list

import (
	"fmt"
	"os"
	"strconv"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// LLMOutput is the JSON representation of an LLM for --output-format json.
// Source discriminates gateway catalog models from DataRobot-deployed LLMs;
// DeploymentID is set only for deployed entries and is what downstream tooling
// uses to write the deployed-model .env keys.
type LLMOutput struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Source       string `json:"source"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Description  string `json:"description"`
	ContextSize  int    `json:"context_size"`
	DeploymentID string `json:"deployment_id"`
	Selected     bool   `json:"selected"`
}

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	cmd := &cobra.Command{
		Use:          "list",
		Aliases:      []string{"ls"},
		Short:        "List available LLMs",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		PreRunE:      auth.EnsureAuthenticatedE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			llmList, err := drapi.GetLLMsAndDeployed()
			if err != nil {
				return err
			}

			selectedID := viperx.GetString(config.DefaultLLMID)

			format := outputformat.GetFormat(cmd)
			if format == outputformat.OutputFormatJSON {
				outputs := toLLMOutputs(llmList.LLMs, selectedID)

				return outputformat.PrintJSONEnvelope(os.Stdout, "llms", outputs)
			}

			printLLMTable(llmList.LLMs, selectedID)

			return nil
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"output_format": string(outputFormat),
		}
	})

	return cmd
}

func toLLMOutputs(llms []drapi.LLM, selectedID string) []LLMOutput {
	outputs := make([]LLMOutput, len(llms))

	for i, l := range llms {
		outputs[i] = LLMOutput{
			ID:           l.LlmID,
			Name:         l.Name,
			Source:       l.Kind,
			Provider:     l.Provider,
			Model:        l.Model,
			Description:  l.Description,
			ContextSize:  l.ContextSize,
			DeploymentID: l.DeploymentID,
			Selected:     l.LlmID == selectedID,
		}
	}

	return outputs
}

func terminalWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 120
	}

	return w
}

// formatContextSize renders a context-window size for the table. A zero or
// missing value shows as "-" so it reads as unknown, not a real zero-token limit.
func formatContextSize(n int) string {
	if n <= 0 {
		return "-"
	}

	return strconv.Itoa(n)
}

func printLLMTable(llms []drapi.LLM, selectedID string) {
	fmt.Println(tui.SubTitleStyle.Render("Available LLMs"))

	idStyle := tui.BaseTextStyle.
		Foreground(tui.GetAdaptiveColor(tui.DrPurple, tui.DrPurpleDark)).
		Padding(0, 1)

	nameStyle := tui.BaseTextStyle.
		Padding(0, 1)

	dimStyle := tui.DimStyle.
		Padding(0, 1)

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tui.TableBorderStyle).
		StyleFunc(func(_, col int) lipgloss.Style {
			switch col {
			case 0:
				return idStyle
			case 1:
				return nameStyle
			default:
				return dimStyle
			}
		}).
		Headers("ID", "NAME", "SOURCE", "PROVIDER", "MODEL", "CONTEXT")

	for _, l := range llms {
		id := "  " + l.LlmID
		if l.LlmID == selectedID {
			id = "* " + l.LlmID
		}

		// Deployed rows carry no provider and only the litellm sentinel model;
		// both are noise in the table (the SOURCE column already says
		// "deployed"), so show "-" and keep the sentinel in JSON output only.
		provider, model := l.Provider, l.Model
		if l.Kind == drapi.LLMKindDeployed {
			provider, model = "-", "-"
		}

		t.Row(id, l.Name, l.Kind, provider, model, formatContextSize(l.ContextSize))
	}

	rendered := t.Render()
	if lipgloss.Width(rendered) > terminalWidth() {
		rendered = t.Width(terminalWidth()).Render()
	}

	_, _ = fmt.Fprintln(os.Stdout, rendered)
}
