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

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

// TemplateOutput is the JSON representation of a DataRobot AI application template for --output-format json.
type TemplateOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	cmd := &cobra.Command{
		Use:   "list",
		Short: "📋 List all available AI application templates",
		Long: `List all available AI application templates from DataRobot.

This command shows you all the pre-built templates you can use to quickly
start building AI applications. Each template includes:
  • Complete application structure
  • Pre-configured components
  • Documentation and examples
  • Ready-to-deploy setup

💡 Use 'dr templates setup' for an interactive selection experience.`,
		PreRunE: auth.EnsureAuthenticatedE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			templateList, err := drapi.GetTemplates()
			if err != nil {
				return err
			}

			format := outputformat.GetFormat(cmd)
			if format == outputformat.OutputFormatJSON {
				outputs := make([]TemplateOutput, len(templateList.Templates))
				for i, t := range templateList.Templates {
					outputs[i] = TemplateOutput{ID: t.ID, Name: t.Name}
				}

				return outputformat.PrintJSONEnvelope(os.Stdout, "templates", outputs)
			}

			if len(templateList.Templates) == 0 {
				fmt.Println("No templates available.")
				return nil
			}

			idStyle := tui.DimStyle.
				Padding(0, 1)

			nameStyle := tui.BaseTextStyle.
				Foreground(tui.GetAdaptiveColor(tui.DrPurple, tui.DrPurpleDark)).
				Padding(0, 1)

			t := table.New().
				Border(lipgloss.RoundedBorder()).
				BorderStyle(tui.TableBorderStyle).
				StyleFunc(func(_, col int) lipgloss.Style {
					switch col {
					case 0:
						return idStyle
					default:
						return nameStyle
					}
				}).
				Headers("ID", "NAME")

			for _, template := range templateList.Templates {
				t.Row(template.ID, template.Name)
			}

			_, _ = fmt.Fprintln(os.Stdout, t.Render())

			return nil
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	return cmd
}
