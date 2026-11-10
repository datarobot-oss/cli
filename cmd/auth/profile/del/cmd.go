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
	"errors"
	"fmt"

	authcompletion "github.com/datarobot/cli/cmd/auth/profile/completion"
	"github.com/datarobot/cli/cmd/helpers"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/tui"
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

Without --yes the command asks for confirmation. The deletion removes the
profile's stored API token, which cannot be recovered without logging in
again.

Example:
  dr auth profile delete staging
  dr auth profile rm staging --yes`,
		Args: cobra.ExactArgs(1),
		RunE: runDelete,
		// Offer only the deletable (non-reserved) profile names for the one
		// positional argument: "default" is never suggested because deleting
		// it is always refused.
		ValidArgsFunction: authcompletion.PositionalCompletion(false),
	}

	cmd.Flags().BoolP(cli.YesFlagName, "y", false, "Skip the confirmation prompt.")

	// Bind only the env var to viper. The --yes flag itself is read directly
	// from cmd.Flags() so an explicit --yes does not leak into
	// viper.AllSettings() and persist to drconfig.yaml.
	_ = viperx.BindEnv(cli.YesFlagName, reader.NonInteractiveEnv)

	return cmd
}

// runDelete hands the raw name to the config package's comment-preserving
// YAML-node editor, which owns the normalization (matching --profile
// selection semantics). The prompt and confirmation use the same idempotent
// normalization so the user sees the name that would go, and the one that
// went.
func runDelete(cmd *cobra.Command, args []string) error {
	format := outputformat.GetFormat(cmd)
	name := config.NormalizeProfileName(args[0])

	confirmed, err := confirmDelete(cmd, name)
	if err != nil {
		return err
	}

	if !confirmed {
		return renderDeletion(cmd, format, name, false, deletionReasonAborted)
	}

	if err := config.DeleteProfile(args[0]); err != nil {
		return err
	}

	return renderDeletion(cmd, format, name, true, "")
}

// confirmDelete returns (true, nil) when the deletion may proceed: either
// --yes / DATAROBOT_CLI_NON_INTERACTIVE was given, or the user confirmed
// interactively. A declined prompt is (false, nil) so the command exits 0
// without touching the file, the same contract the artifact and enclave
// delete verbs follow.
func confirmDelete(cmd *cobra.Command, name string) (bool, error) {
	if cli.IsNonInteractive(cmd) {
		return true, nil
	}

	if !reader.IsStdinTerminal() {
		return false, errors.New("confirmation required: pass --yes (or set DATAROBOT_CLI_NON_INTERACTIVE=1) to delete without a prompt")
	}

	confirmed, err := helpers.Confirm(cmd.OutOrStdout(), cmd.InOrStdin(),
		fmt.Sprintf("Delete profile %q? [y/N] ", name))

	return confirmed, err
}

// deletionReasonAborted marks the JSON envelope of a declined confirmation.
const deletionReasonAborted = "aborted"

// renderDeletion reports the outcome once. Text mode prints the
// confirmation, or the abort note for a declined prompt; --output-format
// json replaces both with a machine-readable envelope so stdout stays a
// single parseable document. An abort exits 0 as a no-op, distinguished in
// JSON by "deleted": false and the reason.
func renderDeletion(cmd *cobra.Command, format outputformat.OutputFormat, name string, deleted bool, reason string) error {
	if format == outputformat.OutputFormatJSON {
		payload := map[string]any{
			"name":    name,
			"deleted": deleted,
		}

		if reason != "" {
			payload["reason"] = reason
		}

		return outputformat.PrintJSONEnvelope(cmd.OutOrStdout(), "profile", payload)
	}

	if !deleted {
		fmt.Fprintln(cmd.OutOrStdout(), tui.DimStyle.Render("Aborted."))

		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Deleted profile %q.\n", name)

	return nil
}
