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

// Package del implements the `dr auth profile delete` verb (alias `rm`).
// The directory is named `del` rather than `delete` because the latter
// shadows Go's built-in delete() function in importing files, the same
// convention the workload and pipeline delete verbs follow.
package del

import (
	"fmt"

	"github.com/datarobot/cli/internal/config"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <name>",
		Aliases: []string{"rm"},
		Short:   "❌ Delete a named profile from drconfig.yaml",
		Long: `Delete a named profile's section from drconfig.yaml.

Only the named section is removed: the root-level credentials, every other
profile, global settings, and unrelated comments are kept. Names are matched
like --profile names, so " BETA " and "DEFAULT" case-normalize onto their
sections.

The reserved name "default" cannot be deleted: it addresses the root-level
credentials whenever no explicit profiles.default section is defined, and
those are cleared by 'dr auth logout' instead.

The profile is deleted immediately; there is no confirmation prompt.`,
		Args: cobra.ExactArgs(1),
		RunE: runDelete,
	}

	return cmd
}

// runDelete normalizes the requested name, hands the deletion to the config
// package's comment-preserving YAML-node editor, and confirms the normalized
// name of the profile actually removed.
func runDelete(cmd *cobra.Command, args []string) error {
	name := config.NormalizeProfileName(args[0])

	if err := config.DeleteProfile(name); err != nil {
		return err
	}

	fmt.Printf("Deleted profile %q.\n", name)

	return nil
}
