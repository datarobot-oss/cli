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

package allcommands

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "all-commands",
		Short: "Display all available commands and their flags, in tree format.",
		Long:  "Display all available commands, subcommands, and their flags, in a tree format.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			output := GenerateCommandTree(cmd.Root())

			_, _ = fmt.Fprint(cmd.OutOrStdout(), output)

			return nil
		},
	}

	return cmd
}

// GenerateCommandTree generates a tree representation of all commands and flags
func GenerateCommandTree(rootCmd *cobra.Command) string {
	var builder strings.Builder

	builder.WriteString(rootCmd.Name() + "\n")

	// Get all commands and sort them
	commands := rootCmd.Commands()
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Name() < commands[j].Name()
	})

	for i, subCmd := range commands {
		isLast := i == len(commands)-1

		printCommand(&builder, subCmd, "", isLast)
	}

	return builder.String()
}

func printCommand(builder *strings.Builder, cmd *cobra.Command, prefix string, isLast bool) {
	// Determine the tree characters
	var connector, childPrefix string

	if isLast {
		connector = "└── "
		childPrefix = prefix + "    "
	} else {
		connector = "├── "
		childPrefix = prefix + "│   "
	}

	// Print the command name
	fmt.Fprintf(builder, "%s%s%s\n", prefix, connector, cmd.Name())

	// Print flags for this command
	flags := collectFlags(cmd)
	if len(flags) > 0 {
		for i, flag := range flags {
			isFlagLast := i == len(flags)-1

			var flagConnector string

			if isFlagLast && !cmd.HasSubCommands() {
				flagConnector = "└── "
			} else {
				flagConnector = "├── "
			}

			fmt.Fprintf(builder, "%s%s%s\n", childPrefix, flagConnector, flag)
		}
	}

	// Print subcommands recursively
	subCommands := cmd.Commands()
	sort.Slice(subCommands, func(i, j int) bool {
		return subCommands[i].Name() < subCommands[j].Name()
	})

	for i, subCmd := range subCommands {
		isSubLast := i == len(subCommands)-1

		printCommand(builder, subCmd, childPrefix, isSubLast)
	}
}

func collectFlags(cmd *cobra.Command) []string {
	var flags []string

	// Collect local flags (non-inherited)
	cmd.LocalFlags().VisitAll(func(flag *pflag.Flag) {
		flagStr := formatFlag(flag)

		flags = append(flags, flagStr)
	})

	// Sort flags alphabetically
	sort.Strings(flags)

	return flags
}

func formatFlag(flag *pflag.Flag) string {
	var parts []string

	// Add shorthand if it exists
	if flag.Shorthand != "" {
		parts = append(parts, "-"+flag.Shorthand)
	}

	// Add full flag name
	parts = append(parts, "--"+flag.Name)

	// Add type if not bool
	if flag.Value.Type() != "bool" {
		parts = append(parts, fmt.Sprintf("<%s>", flag.Value.Type()))
	}

	// Add usage description
	flagStr := strings.Join(parts, ", ")

	if flag.Usage != "" {
		flagStr = fmt.Sprintf("%s: %s", flagStr, flag.Usage)
	}

	return flagStr
}
