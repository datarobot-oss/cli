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

// Package del implements the `dr workload delete` verb. The directory is
// named `del` rather than `delete` because the latter shadows Go's built-in
// delete() function in importing files.
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
		Use:   "delete <workload-id>",
		Short: "Delete a workload.",
		Long: `Delete a workload by id.

Deleting a running workload is allowed: the platform stops the backing
replicas first, then removes the workload. The artifact it was created from
is not deleted with it; remove that separately with
'dr artifact delete <artifact-id>' once no workload references it.

Without --yes the command asks for confirmation.

Example:
  dr workload delete 68b0c1d2e3f4a5b6c7d8e9f0
  dr workload delete 68b0c1d2e3f4a5b6c7d8e9f0 --yes`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			confirmed, err := confirmDelete(cmd, args[0])
			if err != nil || !confirmed {
				return err
			}

			if err := workload.DeleteWorkload(args[0]); err != nil {
				return handleDeleteError(err, args[0])
			}

			fmt.Println(tui.BaseTextStyle.Render("Deleted workload: " + args[0]))

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
			"workload_id": telemetry.FirstArg(args),
			"yes":         yesFlag || viperx.GetBool("yes"),
		}
	})

	return cmd
}

// confirmDelete returns (true, nil) when the deletion may proceed: either
// --yes / DATAROBOT_CLI_NON_INTERACTIVE was given, or the user confirmed
// interactively. A declined prompt is (false, nil) so the command exits 0
// as a no-op.
func confirmDelete(cmd *cobra.Command, workloadID string) (bool, error) {
	yesFlag, _ := cmd.Flags().GetBool("yes")
	if yesFlag || viperx.GetBool("yes") {
		return true, nil
	}

	if !reader.IsStdinTerminal() {
		return false, errors.New("confirmation required: pass --yes (or set DATAROBOT_CLI_NON_INTERACTIVE=1) to delete without a prompt")
	}

	confirmed, err := helpers.Confirm(cmd.OutOrStdout(), cmd.InOrStdin(),
		"Delete workload "+workloadID+"? This stops and removes a running workload. [y/N] ")
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
// for what is effectively a no-op.
func handleDeleteError(err error, workloadID string) error {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		fmt.Println(tui.DimStyle.Render("No workload found with id: " + workloadID))

		return nil
	}

	return err
}
