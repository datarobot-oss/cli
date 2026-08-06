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

package grant

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
		role   string
		user   string
		userID string
		group  string
		org    string
	)

	cmd := &cobra.Command{
		Use:   "grant <enclave-id> --role <owner|user|consumer> <recipient>",
		Short: "Grant a role on an enclave to a recipient.",
		Long: `Grant a role on an enclave to a single recipient.

The role is one of:
  owner      full control, including sharing and deletion
  user       deploy workloads and view the enclave
  consumer   view only

Choose exactly one recipient:
  --user <username>   a user, by username
  --user-id <id>      a user, by DataRobot user id
  --group <id>        a group
  --org <id>          an entire organization (all its users)

There is no separate "all users in a tenant" subject; grant the tenant's
organization with --org to cover every user in it.

Takes effect only with ENCLAVE_RBAC_ENABLED=true on the server; otherwise the
call succeeds but grants nothing. The caller must hold CAN_SHARE on the
enclave; system administrators may manage any enclave.

Example:
  dr enclave access grant 3fa85f64-5717-4562-b3fc-2c963f66afa6 --role user --user alice@corp.io
  dr enclave access grant 3fa85f64-5717-4562-b3fc-2c963f66afa6 --role user --org 656f0000000000000000abcd`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			serverRole, err := enclave.ParseRole(role)
			if err != nil {
				return err
			}

			recipient, err := enclave.ResolveRecipient(user, userID, group, org)
			if err != nil {
				return err
			}

			if err := enclave.GrantAccess(args[0], serverRole, recipient); err != nil {
				return err
			}

			return enclave.RenderAccessChange(outputFormat, enclave.AccessChange{
				EnclaveID:     args[0],
				Action:        "granted",
				Role:          serverRole,
				RecipientType: recipient.Type,
				Recipient:     recipient.Value(),
			})
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().StringVar(&role, "role", "", "Role to grant: owner, user, or consumer (required)")
	_ = cmd.MarkFlagRequired("role")

	cmd.Flags().StringVar(&user, "user", "", "Grant a user by username")
	cmd.Flags().StringVar(&userID, "user-id", "", "Grant a user by DataRobot user id")
	cmd.Flags().StringVar(&group, "group", "", "Grant a group by id")
	cmd.Flags().StringVar(&org, "org", "", "Grant an entire organization by id (all its users)")
	cmd.MarkFlagsMutuallyExclusive("user", "user-id", "group", "org")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		recipientType, _ := enclave.RecipientTypeOf(user, userID, group, org)

		return map[string]any{
			"enclave_id":     telemetry.FirstArg(args),
			"role":           role,
			"recipient_type": recipientType,
			"output_format":  string(outputFormat),
		}
	})

	return cmd
}
