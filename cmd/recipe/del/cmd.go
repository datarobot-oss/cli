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

// Package del implements `dr recipe delete`. The directory is named `del`
// rather than `delete` because the latter shadows Go's built-in delete().
package del

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/datarobot/cli/cmd/helpers"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/llm"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	cmd := &cobra.Command{
		Use:   "delete <recipe-id>",
		Short: "Delete a recipe.",
		Long: `Delete a recipe. Only its creator or a tenant admin can.

Workloads already scheduled keep running on the version they resolved; a
deleted recipe cannot be deployed again. Without --yes the command asks for
confirmation. With --output-format json, stdout carries only the result and
the prompt goes to stderr.

Example:
  dr recipe delete 6a1f0c2e9b7d4e3f2a1b0c9d
  dr recipe delete 6a1f0c2e9b7d4e3f2a1b0c9d --yes`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)
			jsonOut := outputFormat == outputformat.OutputFormatJSON

			confirmed, err := confirm(cmd, args[0], jsonOut)
			if err != nil || !confirmed {
				return err
			}

			if err := llm.DeleteRecipe(args[0]); err != nil {
				return handleError(cmd, err, args[0], jsonOut)
			}

			if jsonOut {
				return outputformat.PrintJSONEnvelope(cmd.OutOrStdout(), "deleted", map[string]string{"id": args[0]})
			}

			fmt.Fprintln(cmd.OutOrStdout(), tui.BaseTextStyle.Render("Deleted recipe: "+args[0]))

			return nil
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	cmd.Flags().BoolP(cli.YesFlagName, "y", false, "Skip the confirmation prompt.")

	// Bind only the env var to viper, as `dr artifact delete` does, so an
	// explicit --yes does not persist to drconfig.yaml.
	_ = viperx.BindEnv(cli.YesFlagName, "DATAROBOT_CLI_NON_INTERACTIVE")

	telemetry.TrackWith(cmd, func(cmd *cobra.Command, _ []string) map[string]any {
		return map[string]any{"yes": cli.IsNonInteractive(cmd), "output_format": string(outputFormat)}
	})

	return cmd
}

// confirm asks before deleting. In JSON mode the prompt and "Aborted." go to
// stderr so stdout stays pure JSON.
func confirm(cmd *cobra.Command, id string, jsonOut bool) (bool, error) {
	if cli.IsNonInteractive(cmd) {
		return true, nil
	}

	if !reader.IsStdinTerminal() {
		return false, errors.New("confirmation required: pass --yes (or set DATAROBOT_CLI_NON_INTERACTIVE=1) to delete without a prompt")
	}

	talk := cmd.OutOrStdout()
	if jsonOut {
		talk = cmd.ErrOrStderr()
	}

	ok, err := helpers.Confirm(talk, cmd.InOrStdin(), "Delete recipe "+id+"? [y/N] ")
	if err != nil {
		return false, err
	}

	if !ok {
		fmt.Fprintln(talk, tui.DimStyle.Render("Aborted."))
	}

	return ok, nil
}

// handleError turns a 404 into an informational line (exit 0), as
// `dr artifact delete` does; JSON mode keeps the error so stdout stays pure
// JSON, as `dr recipe get` does. Everything else keeps the server's detail.
func handleError(cmd *cobra.Command, err error, id string, jsonOut bool) error {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound && !jsonOut {
		fmt.Fprintln(cmd.OutOrStdout(), tui.DimStyle.Render("No recipe found with id: "+id))

		return nil
	}

	return err
}
