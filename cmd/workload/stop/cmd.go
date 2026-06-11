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

package stop

import (
	"fmt"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat workload.OutputFormat

	cmd := &cobra.Command{
		Use:   "stop <workload-id>",
		Short: "Stop a workload.",
		Long: `Stop a workload.

The stop is asynchronous: the server acknowledges the request and the
workload transitions stopping → stopped in the background. Stopping an
already-stopped workload is a no-op; the server's response message says
so. The workload is not deleted and can be brought back with
'dr workload start <workload-id>'.

The acknowledgement message is printed on stdout. Use
'dr workload status <workload-id>' to check the workload's progress.

By default, output is human-readable. Use --output-format json for the
full acknowledgement document.

Example:
  dr workload stop 68b0c1d2e3f4a5b6c7d8e9f0
  dr workload stop 68b0c1d2e3f4a5b6c7d8e9f0 --output-format json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := workload.StopWorkload(args[0])
			if err != nil {
				return err
			}

			if err := workload.RenderWorkloadOperation(outputFormat, *resp); err != nil {
				return err
			}

			// The follow-up hint goes to stderr so script captures of stdout
			// stay limited to the server's acknowledgement message.
			if outputFormat == workload.OutputFormatText {
				fmt.Fprintln(cmd.ErrOrStderr(), "Check progress with: dr workload status "+args[0])
			}

			return nil
		},
	}

	workload.AddOutputFlag(cmd, &outputFormat)

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"workload_id":   telemetry.FirstArg(args),
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
