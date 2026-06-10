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

package describe

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/datarobot/cli/cmd/component/shared"
	"github.com/datarobot/cli/internal/appframework"
	"github.com/datarobot/cli/internal/tools"
	"github.com/spf13/cobra"
)

type describeOptions struct {
	format string
}

func PreRunE(_ *cobra.Command, _ []string) error {
	if err := tools.CheckPrerequisite("uv"); err != nil {
		return err
	}

	return nil
}

func RunE(opts *describeOptions) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, args []string) error {
		fw := shared.GetFrameworkPath()
		query := args[0]

		modules, err := appframework.DescribeFramework(fw, ".")
		if err != nil {
			return err
		}

		m, found := findModule(modules, query)
		if !found {
			return fmt.Errorf("module %q not found; run `dr component list-available` to see available modules", query)
		}

		if opts.format == "json" {
			b, err := json.MarshalIndent(m, "", "  ")
			if err != nil {
				return fmt.Errorf("marshalling module to JSON: %w", err)
			}

			fmt.Println(string(b))

			return nil
		}

		printModuleText(m)

		return nil
	}
}

// findModule locates a module by short name (e.g. "agent") or disambiguated name (e.g. "core.agent").
func findModule(modules []appframework.Module, query string) (appframework.Module, bool) {
	for _, m := range modules {
		if m.DisambiguatedName == query || m.Name == query {
			return m, true
		}
	}

	// Suffix fallback: match if the disambiguated name ends with ".<query>".
	suffix := "." + query

	for _, m := range modules {
		if strings.HasSuffix(m.DisambiguatedName, suffix) {
			return m, true
		}
	}

	return appframework.Module{}, false
}

func printModuleText(m appframework.Module) {
	fmt.Printf("Module:       %s (%s)\n", m.Name, m.DisambiguatedName)
	fmt.Printf("Display name: %s\n", m.DisplayName)
	fmt.Printf("Description:  %s\n", m.Description)

	if len(m.Dependencies) > 0 {
		fmt.Printf("Dependencies: %s\n", strings.Join(m.Dependencies, ", "))
	} else {
		fmt.Printf("Dependencies: none\n")
	}

	if m.Repeatable != nil {
		fmt.Printf("Repeatable:   yes (%s)\n", *m.Repeatable)
	} else {
		fmt.Printf("Repeatable:   no\n")
	}

	if m.AgentGuidance != nil && m.AgentGuidance.Summary != "" {
		fmt.Printf("\nAgent guidance: %s\n", m.AgentGuidance.Summary)
	}

	if len(m.Questions) == 0 {
		return
	}

	fmt.Printf("\nQuestions:\n")

	for _, q := range m.Questions {
		askUser := false
		reason := ""

		if q.AgentGuidance != nil {
			askUser = q.AgentGuidance.AskUser
			reason = q.AgentGuidance.Reason
		}

		required := q.Default == nil

		defaultStr := "required"

		if !required {
			defaultStr = fmt.Sprintf("default=%v", q.Default)
		}

		fmt.Printf("  %s  [%s, %s, ask_user=%v]\n", q.Name, q.Type, defaultStr, askUser)

		if q.Help != "" {
			fmt.Printf("    Help:     %s\n", q.Help)
		}

		if reason != "" {
			fmt.Printf("    Guidance: %s\n", reason)
		}

		if len(q.Choices) > 0 {
			choiceStrs := make([]string, 0, len(q.Choices))

			for _, c := range q.Choices {
				choiceStrs = append(choiceStrs, fmt.Sprintf("%v", c))
			}

			fmt.Printf("    Choices:  %s\n", strings.Join(choiceStrs, " | "))
		}
	}
}

func Cmd() *cobra.Command {
	opts := &describeOptions{
		format: "text",
	}

	cmd := &cobra.Command{
		Use:     "describe <module>",
		Short:   "🔍 Describe an available component module",
		Args:    cobra.ExactArgs(1),
		PreRunE: PreRunE,
		RunE:    RunE(opts),
	}

	cmd.Flags().StringVar(
		&opts.format,
		"format",
		"text",
		"Output format (options: json, text)",
	)

	_ = cmd.RegisterFlagCompletionFunc("format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "text"}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}
