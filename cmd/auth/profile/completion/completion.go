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

// Package completion provides the shell-completion functions for auth profile
// names: the value of the global --profile flag and the single positional
// name accepted by `auth profile show` and `auth profile delete` (alias rm).
//
// Candidates are the explicitly defined profile sections of the config file
// selected for the completion invocation (--config flag, DATAROBOT_CLI_CONFIG,
// or the default location). The functions read that file in isolation — never
// through the global config layer — so completion works without requiring the
// normal pre-run's authentication checks, without mutating any configuration,
// and without any network request. Any read problem yields no candidates:
// error text must never land in the shell's candidate list.
//
// The reserved "default" name is offered only when an explicit profiles.default
// section is defined. Deletion suggestions drop it entirely because deleting
// it is always refused (see `auth profile delete`); the positional
// `auth profile show default` root fallback still works even when the section
// is absent and therefore not suggested.
package completion

import (
	"strings"

	"github.com/datarobot/cli/internal/config"
	"github.com/spf13/cobra"
)

// FlagCompletion completes the value of the global --profile flag with the
// explicitly defined profile names. It is registered once on the root's
// persistent flag, which every subcommand inherits. File completion is
// disabled: only defined profile names are valid values.
func FlagCompletion(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeProfileNames(cmd, toComplete, false)
}

// PositionalCompletion returns the completion function for the single name
// argument of show/delete. Candidates are offered only before the first
// argument (both commands accept at most one name) and are prefix-filtered,
// with file completion disabled. includeDefault controls whether an explicit
// profiles.default section is suggested: show keeps it, delete omits it.
func PositionalCompletion(includeDefault bool) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// One name is all either command accepts, so once one is typed
		// there is nothing left to suggest.
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		return completeProfileNames(cmd, toComplete, !includeDefault)
	}
}

// completeProfileNames loads the explicit profile names from the config
// selected for this completion invocation, filters them by the typed prefix,
// and reports the no-file-completion directive.
func completeProfileNames(cmd *cobra.Command, toComplete string, omitDefault bool) ([]string, cobra.ShellCompDirective) {
	candidates := profileCandidates(cmd, omitDefault)

	matches := make([]string, 0, len(candidates))

	for _, name := range candidates {
		if strings.HasPrefix(name, toComplete) {
			matches = append(matches, name)
		}
	}

	return matches, cobra.ShellCompDirectiveNoFileComp
}

// profileCandidates resolves the config file for the completion invocation
// and returns its defined profile names, dropping the reserved "default" when
// omitDefault is set. Any read problem yields no candidates.
func profileCandidates(cmd *cobra.Command, omitDefault bool) []string {
	explicitConfig, err := cmd.Flags().GetString("config")
	if err != nil {
		// The --config flag is always registered on the root, so this only
		// guards commands built without it; an empty value falls through to
		// env/default resolution.
		explicitConfig = ""
	}

	names, err := config.ProfileNamesFromFile(config.CompletionConfigPath(explicitConfig))
	if err != nil {
		return nil
	}

	if !omitDefault {
		return names
	}

	filtered := make([]string, 0, len(names))

	for _, name := range names {
		if name != config.DefaultProfileLabel {
			filtered = append(filtered, name)
		}
	}

	return filtered
}
