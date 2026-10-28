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

package list

import (
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/enclave"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	cmd := &cobra.Command{
		Use:     "list <enclave-id>",
		Aliases: []string{"ls"},
		Short:   "List who has access to an enclave.",
		Long: `List the users, groups, and organizations an enclave is shared with, and the
role each holds.

This is the "did my grant land?" view. "dr enclave access show" answers only for
a single subject, and for a system administrator it answers from the bypass
(always the full set), so neither reveals what the enclave is actually shared
with.

An empty result means nothing has been shared yet — including the case where the
enclave was registered before enclave RBAC was enabled and so never received its
owner grant.

Requires CAN_VIEW on the enclave; system administrators may list any enclave.

Example:
  dr enclave access list 3fa85f64-5717-4562-b3fc-2c963f66afa6
  dr enclave access list 3fa85f64-5717-4562-b3fc-2c963f66afa6 --output-format json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			roles, err := enclave.ListSharedRoles(args[0])
			if err != nil {
				return err
			}

			return enclave.RenderSharedRoles(outputFormat, roles)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"enclave_id":    telemetry.FirstArg(args),
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
