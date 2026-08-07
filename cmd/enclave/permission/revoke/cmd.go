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

package revoke

import (
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/enclave"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	var (
		permission string
		userID     string
		group      string
		org        string
	)

	cmd := &cobra.Command{
		Use:   "revoke --permission create <recipient>",
		Short: "Revoke a collection-level enclave permission from a recipient.",
		Long: `Revoke a collection-level enclave permission from a single recipient.

The permission is:
  create   register new enclaves

Choose exactly one recipient:
  --user-id <id>   a user, by DataRobot user id
  --group <id>     a group
  --org <id>       an entire organization

Recipients are named by id only — unlike "dr enclave access", there is no
--user <username> form. Revoking a recipient's permission does not affect
enclaves they already own, nor any permission granted to them by way of a
different subject (for example, an organization-wide grant).

Takes effect only with ENCLAVE_RBAC_ENABLED=true on the server; otherwise the
call succeeds but changes nothing. Revoking requires a system administrator.

Example:
  dr enclave permission revoke --permission create --org 656f0000000000000000abcd
  dr enclave permission revoke --permission create --user-id 656f0000000000000000abce`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			name, err := enclave.ParsePermission(permission)
			if err != nil {
				return err
			}

			recipient, err := enclave.ResolveRecipientByID(userID, group, org)
			if err != nil {
				return err
			}

			if err := enclave.RevokeCreatePermission(recipient); err != nil {
				return err
			}

			return enclave.RenderPermissionChange(outputFormat, enclave.PermissionChange{
				Permission:    name,
				Action:        "revoked",
				RecipientType: recipient.Type,
				Recipient:     recipient.Value(),
			})
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().StringVar(&permission, "permission", "", "Permission to revoke: create (required)")
	_ = cmd.MarkFlagRequired("permission")

	cmd.Flags().StringVar(&userID, "user-id", "", "Revoke from a user by DataRobot user id")
	cmd.Flags().StringVar(&group, "group", "", "Revoke from a group by id")
	cmd.Flags().StringVar(&org, "org", "", "Revoke from an entire organization by id")
	cmd.MarkFlagsMutuallyExclusive("user-id", "group", "org")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		recipientType, _ := enclave.RecipientTypeByIDOf(userID, group, org)

		return map[string]any{
			"permission":     permission,
			"recipient_type": recipientType,
			"output_format":  string(outputFormat),
		}
	})

	return cmd
}
