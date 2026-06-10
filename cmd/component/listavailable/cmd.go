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

package listavailable

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/datarobot/cli/cmd/component/shared"
	"github.com/datarobot/cli/internal/appframework"
	"github.com/datarobot/cli/internal/tools"
	"github.com/spf13/cobra"
)

type listAvailableOptions struct {
	format string
}

func PreRunE(_ *cobra.Command, _ []string) error {
	if err := tools.CheckPrerequisite("uv"); err != nil {
		return err
	}

	return nil
}

func ensureReady(fw string) error {
	if err := appframework.ExecInitializeFramework(fw); err != nil {
		return fmt.Errorf("initializing framework: %w", err)
	}

	aliases, err := appframework.RegistryAliases(fw, ".")
	if err != nil {
		return err
	}

	if aliases["core"] {
		return nil
	}

	fmt.Println("Adding default registry...")

	return appframework.ExecAddRegistry(appframework.RegistryURI(), "core", fw)
}

func RunE(opts *listAvailableOptions) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, _ []string) error {
		fw := shared.GetFrameworkPath()

		if err := ensureReady(fw); err != nil {
			return err
		}

		modules, err := appframework.ListAvailableComponents(fw, ".")
		if err != nil {
			return err
		}

		if len(modules) == 0 {
			fmt.Println("No components found. Ensure a registry is registered.")

			return nil
		}

		if opts.format == "json" {
			b, err := json.MarshalIndent(modules, "", "  ")
			if err != nil {
				return fmt.Errorf("marshalling modules to JSON: %w", err)
			}

			fmt.Println(string(b))

			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

		fmt.Fprintf(w, "NAME\tDISPLAY NAME\tDESCRIPTION\n")

		for _, m := range modules {
			desc := m.Description
			repeatable := ""

			if m.Repeatable != nil {
				repeatable = " [repeatable]"
			}

			fmt.Fprintf(w, "%s\t%s\t%s%s\n", m.Name, m.DisplayName, desc, repeatable)
		}

		if err := w.Flush(); err != nil {
			return errors.New("flushing output")
		}

		return nil
	}
}

func Cmd() *cobra.Command {
	opts := &listAvailableOptions{
		format: "text",
	}

	cmd := &cobra.Command{
		Use:     "list-available",
		Short:   "📋 List components available in the registered registry",
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
