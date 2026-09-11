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
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List who may create enclaves.",
		Long: `List the users, groups, and organizations granted the enclave create
permission.

This is the "did my grant land?" view for "dr enclave permission grant".
"dr enclave permission show" answers only for a single subject, and for a system
administrator it answers from the bypass, so neither reveals who has actually
been granted the permission.

An empty result means nobody has been granted it — note that system
administrators may create enclaves regardless and so do not appear here.

Requires a system administrator, matching grant and revoke.

Example:
  dr enclave permission list
  dr enclave permission list --output-format json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			holders, err := enclave.ListCreateAccess()
			if err != nil {
				return err
			}

			return enclave.RenderCreateAccess(outputFormat, holders)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{"output_format": string(outputFormat)}
	})

	return cmd
}
