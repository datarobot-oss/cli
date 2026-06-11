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

// Package del implements the `dr workload artifact delete` verb. The
// directory is named `del` rather than `delete` because the latter shadows
// Go's built-in delete() function in importing files.
package del

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/datarobot/cli/cmd/helpers"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <artifact-id>",
		Short: "Delete a workload artifact",
		Long: `Delete a workload artifact by id.

Two server-side rules apply:
  • Locked artifacts can never be deleted (locking is one-way).
  • An artifact still referenced by a workload cannot be deleted; the error
    names the blocking workload ids. Delete those first with
    'dr workload delete <workload-id>'.

Without --yes the command asks for confirmation.

Example:
  dr workload artifact delete 68b0c1d2e3f4a5b6c7d8e9f0
  dr workload artifact delete 68b0c1d2e3f4a5b6c7d8e9f0 --yes`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			confirmed, err := confirmDelete(cmd, args[0])
			if err != nil || !confirmed {
				return err
			}

			if err := workload.DeleteArtifact(args[0]); err != nil {
				return handleDeleteError(err, args[0])
			}

			fmt.Println(tui.BaseTextStyle.Render("Deleted artifact: " + args[0]))

			return nil
		},
	}

	cmd.Flags().BoolP("yes", "y", false, "Skip the confirmation prompt.")

	// Bind only the env var (DATAROBOT_CLI_NON_INTERACTIVE) to viper. The --yes
	// flag itself is read directly from cmd.Flags() so an explicit --yes does
	// not leak into viper.AllSettings() and persist to drconfig.yaml.
	_ = viperx.BindEnv("yes", "DATAROBOT_CLI_NON_INTERACTIVE")

	telemetry.TrackWith(cmd, func(cmd *cobra.Command, args []string) map[string]any {
		yesFlag, _ := cmd.Flags().GetBool("yes")

		return map[string]any{
			"artifact_id": telemetry.FirstArg(args),
			"yes":         yesFlag || viperx.GetBool("yes"),
		}
	})

	return cmd
}

func confirmDelete(cmd *cobra.Command, artifactID string) (bool, error) {
	yesFlag, _ := cmd.Flags().GetBool("yes")
	if yesFlag || viperx.GetBool("yes") {
		return true, nil
	}

	if !reader.IsStdinTerminal() {
		return false, errors.New("confirmation required: pass --yes (or set DATAROBOT_CLI_NON_INTERACTIVE=1) to delete without a prompt")
	}

	confirmed, err := helpers.Confirm(cmd.OutOrStdout(), cmd.InOrStdin(),
		"Delete artifact "+artifactID+"? [y/N] ")
	if err != nil {
		return false, err
	}

	if !confirmed {
		fmt.Println(tui.DimStyle.Render("Aborted."))
	}

	return confirmed, nil
}

// handleDeleteError converts a 404 into a friendly informational message
// (returns nil) so the user does not see a stack-trace-style HTTP error
// for what is effectively a no-op. Conflicts (409: locked, or referenced by
// a workload) propagate with the server's detail intact.
func handleDeleteError(err error, artifactID string) error {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		fmt.Println(tui.DimStyle.Render("No artifact found with id: " + artifactID))

		return nil
	}

	return err
}
