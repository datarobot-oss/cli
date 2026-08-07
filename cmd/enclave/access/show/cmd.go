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

package show

import (
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/enclave"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	var userID string

	cmd := &cobra.Command{
		Use:   "show <enclave-id>",
		Short: "Show effective permissions on an enclave.",
		Long: `Show the effective permissions you hold on an enclave, and where they come from.

Permissions are the atomic rights the server checks:
  CAN_VIEW     see the enclave
  CAN_UPDATE   deactivate / reactivate it
  CAN_DELETE   delete it
  CAN_SHARE    grant roles on it to others
  CAN_DEPLOY   schedule workloads onto it

An empty list means no access — which is the usual reason a workload fails to
schedule with "not permitted to deploy".

The reported source matters when the list is full: a system administrator and a
server with enclave RBAC disabled both hold everything without any grant, and
that is indistinguishable from genuinely being the owner unless it is spelled
out.

Pass --user-id to inspect another user instead of yourself (for example, to see
why they cannot deploy). That requires a system administrator.

Example:
  dr enclave access show 3fa85f64-5717-4562-b3fc-2c963f66afa6
  dr enclave access show 3fa85f64-5717-4562-b3fc-2c963f66afa6 --output-format json
  dr enclave access show 3fa85f64-5717-4562-b3fc-2c963f66afa6 --user-id 656f0000000000000000abcd`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			permissions, err := enclave.GetEnclavePermissions(args[0], userID)
			if err != nil {
				return err
			}

			return enclave.RenderEnclavePermissions(outputFormat, *permissions)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().StringVar(&userID, "user-id", "",
		"Inspect another user by DataRobot user id (system administrators only)")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"enclave_id":    telemetry.FirstArg(args),
			"for_other":     userID != "",
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
