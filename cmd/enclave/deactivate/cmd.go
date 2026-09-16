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

package deactivate

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
		Use:   "deactivate <enclave-id>",
		Short: "Deactivate an enclave.",
		Long: `Deactivate an enclave (outpost) so it stops accepting new workloads.

The enclave record is preserved; this only changes its status. Use
'dr enclave delete' to remove it entirely.

By default, output is human-readable. Use --output-format json for machine-parseable output.

Example:
  dr enclave deactivate 3fa85f64-5717-4562-b3fc-2c963f66afa6
  dr enclave deactivate 3fa85f64-5717-4562-b3fc-2c963f66afa6 --output-format json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			e, err := enclave.DeactivateEnclave(args[0])
			if err != nil {
				return err
			}

			return enclave.RenderEnclave(outputFormat, *e)
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
