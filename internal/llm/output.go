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

package llm

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/tui"
	"gopkg.in/yaml.v3"
)

const (
	timestampFormat = "2006-01-02 15:04 UTC"
	placeholder     = "—"
)

func orPlaceholder(s string) string {
	if s == "" {
		return placeholder
	}

	return s
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return placeholder
	}

	return t.UTC().Format(timestampFormat)
}

func version(v int) string {
	if v <= 0 {
		return placeholder
	}

	return strconv.Itoa(v)
}

func newTable(headers ...string) *table.Table {
	cell := tui.BaseTextStyle.Padding(0, 1)

	return table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tui.TableBorderStyle).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return cell.Bold(true)
			}

			return cell
		}).
		Headers(headers...)
}

// RenderRecipes prints a list of recipes as a table or a JSON envelope.
func RenderRecipes(w io.Writer, format outputformat.OutputFormat, recipes []Recipe) error {
	if format == outputformat.OutputFormatJSON {
		if recipes == nil {
			recipes = []Recipe{}
		}

		return outputformat.PrintJSONEnvelope(w, "recipes", recipes)
	}

	if len(recipes) == 0 {
		_, err := fmt.Fprintln(w, "No recipes found.")

		return err
	}

	t := newTable("RECIPE ID", "NAME", "KIND", "MODEL", "VERSION", "STATUS", "CREATED BY", "UPDATED")

	for _, r := range recipes {
		t.Row(r.ID, r.Name, orPlaceholder(r.Kind), orPlaceholder(r.ModelName()), version(r.Version),
			orPlaceholder(r.Status), orPlaceholder(r.Creator.Display()), stamp(r.UpdatedAt))
	}

	_, err := fmt.Fprintln(w, t.Render())

	return err
}

// RenderRecipe prints one recipe: its header, validation diagnostics and
// descriptor as text, or the whole recipe as a JSON envelope.
func RenderRecipe(w io.Writer, format outputformat.OutputFormat, r Recipe) error {
	if format == outputformat.OutputFormatJSON {
		return outputformat.PrintJSONEnvelope(w, "recipe", r)
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	fmt.Fprintf(tw, "ID:\t%s\n", r.ID)
	fmt.Fprintf(tw, "Name:\t%s\n", r.Name)
	fmt.Fprintf(tw, "Description:\t%s\n", orPlaceholder(r.Description))
	fmt.Fprintf(tw, "Kind:\t%s\n", orPlaceholder(r.Kind))
	fmt.Fprintf(tw, "Model:\t%s\n", orPlaceholder(r.ModelName()))
	fmt.Fprintf(tw, "Version:\t%s\n", version(r.Version))
	fmt.Fprintf(tw, "Status:\t%s\n", orPlaceholder(r.Status))
	fmt.Fprintf(tw, "Content hash:\t%s\n", orPlaceholder(r.ContentHash))
	fmt.Fprintf(tw, "Created by:\t%s\n", orPlaceholder(r.Creator.Display()))
	fmt.Fprintf(tw, "Updated by:\t%s\n", orPlaceholder(r.UpdatedBy.Display()))
	fmt.Fprintf(tw, "Created:\t%s\n", stamp(r.CreatedAt))
	fmt.Fprintf(tw, "Updated:\t%s\n", stamp(r.UpdatedAt))

	if err := tw.Flush(); err != nil {
		return err
	}

	RenderDiagnostics(w, r)

	if len(r.Descriptor) > 0 && string(r.Descriptor) != "null" {
		out, err := toYAML(r.Descriptor)
		if err != nil {
			return err
		}

		fmt.Fprintf(w, "\nDescriptor:\n%s", indent(out))
	}

	return nil
}

// RenderDiagnostics prints the recipe's validation findings, if any.
func RenderDiagnostics(w io.Writer, r Recipe) {
	diags := r.Diagnostics()
	if len(diags) == 0 {
		return
	}

	fmt.Fprintln(w, "\nDiagnostics:")

	for _, d := range diags {
		where := ""
		if d.Path != "" {
			where = " at " + d.Path
		}

		fmt.Fprintf(w, "  %s %s%s: %s\n", d.Severity, d.Code, where, d.Message)
	}
}

// RenderPayload prints only the recipe's payload: YAML for text, JSON for
// --output-format json. Either form can be edited and passed to patch.
func RenderPayload(w io.Writer, format outputformat.OutputFormat, r Recipe) error {
	if len(r.Payload) == 0 {
		return fmt.Errorf("recipe %s returned no payload", r.ID)
	}

	if format == outputformat.OutputFormatJSON {
		var v any

		if err := json.Unmarshal(r.Payload, &v); err != nil {
			return err
		}

		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")

		return enc.Encode(v)
	}

	out, err := toYAML(r.Payload)
	if err != nil {
		return err
	}

	_, err = io.WriteString(w, out)

	return err
}

// RenderDeployments prints the llm workloads that run a recipe.
func RenderDeployments(w io.Writer, format outputformat.OutputFormat, deployments []Deployment) error {
	if format == outputformat.OutputFormatJSON {
		if deployments == nil {
			deployments = []Deployment{}
		}

		return outputformat.PrintJSONEnvelope(w, "deployments", deployments)
	}

	if len(deployments) == 0 {
		_, err := fmt.Fprintln(w, "No workloads run this recipe.")

		return err
	}

	t := newTable("WORKLOAD ID", "NAME", "STATUS", "RECIPE VERSION", "CREATED")

	for _, d := range deployments {
		t.Row(d.ID, d.Name, orPlaceholder(d.Status), d.RecipeVersion(), stamp(d.CreatedAt))
	}

	_, err := fmt.Fprintln(w, t.Render())

	return err
}

func toYAML(raw json.RawMessage) (string, error) {
	var v any

	if err := json.Unmarshal(raw, &v); err != nil {
		return "", err
	}

	out, err := yaml.Marshal(v)
	if err != nil {
		return "", err
	}

	return string(out), nil
}

func indent(s string) string {
	return "  " + strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n  ") + "\n"
}
