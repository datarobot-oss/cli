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

package get

import (
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat workload.OutputFormat

	cmd := &cobra.Command{
		Use:   "get <workload-id>",
		Short: "Display details of a workload.",
		Long: `Display details of a single workload.

This command fetches a workload by ID and shows:
  • Name and current status
  • Endpoint URL (stable from creation; serves traffic once running)
  • Artifact ID, type, and importance
  • Creation and last update timestamps

Statuses progress submitted → provisioning → launching → running. Re-run
this command until the status is "running", then call the endpoint.

By default, output is human-readable. Use --output-format json for machine-parseable output.

Example:
  dr workload get 68b0c1d2e3f4a5b6c7d8e9f0
  dr workload get 68b0c1d2e3f4a5b6c7d8e9f0 --output-format json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			wl, err := workload.GetWorkload(args[0])
			if err != nil {
				return err
			}

			return workload.RenderWorkload(outputFormat, *wl)
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
