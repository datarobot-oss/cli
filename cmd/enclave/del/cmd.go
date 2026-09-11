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

// Package del implements the `dr enclave delete` verb. The directory is named
// `del` rather than `delete` because the latter shadows Go's built-in delete()
// function in importing files.
package del

import (
	"errors"
	"io"
	"net/http"

	"github.com/datarobot/cli/cmd/helpers"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/enclave"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	cmd := &cobra.Command{
		Use:   "delete <enclave-id>",
		Short: "Delete an enclave.",
		Long: `Delete an enclave (outpost) by id.

This removes the enclave record from your tenant. To take an enclave out of
service without removing it, use 'dr enclave deactivate' instead.

Without --yes the command asks for confirmation.

Deleting an enclave that is already gone, and declining the prompt, are both
no-ops that exit 0. Use --output-format json to tell them apart from a real
deletion: the result carries "deleted" plus a "reason" when it is false.

Example:
  dr enclave delete 3fa85f64-5717-4562-b3fc-2c963f66afa6
  dr enclave delete 3fa85f64-5717-4562-b3fc-2c963f66afa6 --yes
  dr enclave delete 3fa85f64-5717-4562-b3fc-2c963f66afa6 --yes --output-format json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)
			enclaveID := args[0]

			confirmed, err := confirmDelete(cmd, outputFormat, enclaveID)
			if err != nil {
				return err
			}

			if !confirmed {
				return enclave.RenderDeletion(outputFormat, enclave.DeletionResult{
					EnclaveID: enclaveID,
					Reason:    enclave.DeletionReasonAborted,
				})
			}

			if err := enclave.DeleteEnclave(enclaveID); err != nil {
				return handleDeleteError(err, outputFormat, enclaveID)
			}

			return enclave.RenderDeletion(outputFormat, enclave.DeletionResult{
				EnclaveID: enclaveID,
				Deleted:   true,
			})
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	cmd.Flags().BoolP(cli.YesFlagName, "y", false, "Skip the confirmation prompt.")

	// Bind only the env var (DATAROBOT_CLI_NON_INTERACTIVE) to viper. The --yes
	// flag itself is read directly from cmd.Flags() so an explicit --yes does
	// not leak into viper.AllSettings() and persist to drconfig.yaml.
	_ = viperx.BindEnv(cli.YesFlagName, reader.NonInteractiveEnv)

	telemetry.TrackWith(cmd, func(cmd *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"enclave_id":    telemetry.FirstArg(args),
			"yes":           cli.IsNonInteractive(cmd),
			"output_format": string(outputFormat),
		}
	})

	return cmd
}

// confirmDelete returns (true, nil) when the deletion may proceed: either
// --yes / DATAROBOT_CLI_NON_INTERACTIVE was given, or the user confirmed
// interactively. A declined prompt is (false, nil) so the command exits 0
// as a no-op.
func confirmDelete(cmd *cobra.Command, format outputformat.OutputFormat, enclaveID string) (bool, error) {
	if cli.IsNonInteractive(cmd) {
		return true, nil
	}

	if !reader.IsStdinTerminal() {
		return false, errors.New("confirmation required: pass --yes (or set " +
			reader.NonInteractiveEnv + "=1) to delete without a prompt")
	}

	return helpers.Confirm(promptWriter(cmd, format), cmd.InOrStdin(),
		"Delete enclave "+enclaveID+"? [y/N] ")
}

// promptWriter keeps the confirmation prompt off stdout in JSON mode, so the
// only thing a caller parsing stdout sees is the DeletionResult document.
func promptWriter(cmd *cobra.Command, format outputformat.OutputFormat) io.Writer {
	if format == outputformat.OutputFormatJSON {
		return cmd.ErrOrStderr()
	}

	return cmd.OutOrStdout()
}

// handleDeleteError turns a 404 into an already-gone result (exit 0) so the
// user does not see a stack-trace-style HTTP error for what is effectively a
// no-op. Every other error propagates.
func handleDeleteError(err error, format outputformat.OutputFormat, enclaveID string) error {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		return enclave.RenderDeletion(format, enclave.DeletionResult{
			EnclaveID: enclaveID,
			Reason:    enclave.DeletionReasonNotFound,
		})
	}

	return err
}
