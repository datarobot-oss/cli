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

package status

import (
	"github.com/datarobot/cli/cmd/workload/internal/idargs"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	var ref idargs.Ref

	cmd := &cobra.Command{
		Use:   "status [<workload-id>]",
		Short: "Show a workload's status.",
		Long: `Show a workload's current status.

The status is fetched once and printed as a bare value (submitted,
provisioning, launching, running, suspended, interrupted, stopping,
stopped, errored, terminated, or unknown), making the command directly
usable in scripts. An errored status is a valid answer, not a command
failure, so the command still exits zero. Use 'dr workload get' for the
full document.

JSON output emits one {"id", "status"} document.

` + idargs.HelpText + `

Example:
  dr workload status
  dr workload status 68b0c1d2e3f4a5b6c7d8e9f0
  dr workload status 68b0c1d2e3f4a5b6c7d8e9f0 --output-format json`,
		Args:         cobra.MaximumNArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			var err error

			ref, err = idargs.Resolve(cmd, args)
			if err != nil {
				return err
			}

			wl, err := workload.GetWorkload(ref.ID)
			if err != nil {
				return ref.Wrap(err)
			}

			return workload.RenderWorkloadStatus(outputFormat, *wl)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	idargs.AddDirFlag(cmd)

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"workload_id":        ref.ID,
			"workload_id_source": ref.Source,
			"output_format":      string(outputFormat),
		}
	})

	return cmd
}
