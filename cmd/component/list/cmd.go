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
	"github.com/datarobot/cli/internal/copier"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

// ComponentOutput is the JSON representation of an installed component for --output-format json.
type ComponentOutput struct {
	Name string `json:"name"`
	File string `json:"file"`
	Repo string `json:"repo"`
}

func RunE(cmd *cobra.Command, _ []string) error {
	answers, err := copier.AnswersFromPath(".", false)
	if err != nil {
		return err
	}

	format := outputformat.GetFormat(cmd)
	if format == outputformat.OutputFormatJSON {
		outputs := make([]ComponentOutput, len(answers))
		for i, a := range answers {
			outputs[i] = ComponentOutput{
				Name: a.ComponentDetails.Name,
				File: a.FileName,
				Repo: a.Repo,
			}
		}

		return outputformat.PrintJSONEnvelope(os.Stdout, "components", outputs)
	}

	if len(answers) == 0 {
		fmt.Println("No components installed.")
		return nil
	}

	nameStyle := tui.BaseTextStyle.
		Foreground(tui.GetAdaptiveColor(tui.DrPurple, tui.DrPurpleDark)).
		Padding(0, 1)

	fileStyle := tui.DimStyle.
		Padding(0, 1)

	repoStyle := tui.BaseTextStyle.
		Foreground(tui.GetAdaptiveColor(tui.DrPurpleLight, tui.DrPurpleDarkLight)).
		Padding(0, 1)

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tui.TableBorderStyle).
		StyleFunc(func(_, col int) lipgloss.Style {
			switch col {
			case 0:
				return nameStyle
			case 1:
				return fileStyle
			default:
				return repoStyle
			}
		}).
		Headers("NAME", "FILE", "REPO")

	for _, a := range answers {
		t.Row(a.ComponentDetails.Name, a.FileName, a.Repo)
	}

	_, _ = fmt.Fprintln(os.Stdout, t.Render())

	return nil
}

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "📋 List installed components",
		RunE:  RunE,
	}
}
