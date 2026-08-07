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
		Use:   "show",
		Short: "Show your collection-level enclave permissions.",
		Long: `Show whether you may create enclaves, and where that comes from.

Collection-level permissions are not tied to any single enclave. Today there is
one: CAN_CREATE, the right to register a new enclave. Use "dr enclave access
show" for permissions on a specific enclave.

The reported source matters: a system administrator may create enclaves
regardless of grants, and a server with enclave RBAC disabled enforces nothing —
both are indistinguishable from holding CAN_CREATE unless spelled out.

Pass --user-id to inspect another user. That requires a system administrator.

Example:
  dr enclave permission show
  dr enclave permission show --user-id 656f0000000000000000abcd
  dr enclave permission show --output-format json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			permissions, err := enclave.GetCollectionPermissions(userID)
			if err != nil {
				return err
			}

			return enclave.RenderCollectionPermissions(outputFormat, *permissions)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().StringVar(&userID, "user-id", "",
		"Inspect another user by DataRobot user id (system administrators only)")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"for_other":     userID != "",
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
