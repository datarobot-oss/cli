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

package start

import (
	"fmt"

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
		Use:   "start [<workload-id>]",
		Short: "Start a stopped workload.",
		Long: `Start a stopped workload.

The start is asynchronous: the server acknowledges the request and the
workload transitions submitted → provisioning → launching → running in
the background. Starting a workload that is already running, still
initializing, or suspended is a no-op; the server's response message
says so. A workload that is still stopping must finish first (the
server replies with a conflict), and the request is rejected when it
would exceed your concurrent workload limits.

The acknowledgement message is printed on stdout. Use
'dr workload status <workload-id>' to check the workload's progress.

By default, output is human-readable. Use --output-format json for the
full acknowledgement document.

` + idargs.HelpText + `

A workload named by the manifest rather than by you is confirmed
first; pass --yes to skip that.

Example:
  dr workload start
  dr workload start 68b0c1d2e3f4a5b6c7d8e9f0
  dr workload start 68b0c1d2e3f4a5b6c7d8e9f0 --output-format json`,
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

			// Only an ambient target is confirmed: a typed id is the consent.
			if ref.FromManifest() {
				confirmed, err := idargs.Confirm(cmd, idargs.AmbientPrompt("Start", ref), idargs.EnvMayConsent)
				if err != nil || !confirmed {
					return err
				}
			}

			resp, err := workload.StartWorkload(ref.ID)
			if err != nil {
				return ref.Wrap(err)
			}

			if err := workload.RenderWorkloadOperation(outputFormat, *resp); err != nil {
				return err
			}

			// The follow-up hint goes to stderr so script captures of stdout
			// stay limited to the server's acknowledgement message.
			if outputFormat == outputformat.OutputFormatText {
				fmt.Fprintln(cmd.ErrOrStderr(), "Check progress with: dr workload status "+ref.ID)
			}

			return nil
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	idargs.AddDirFlag(cmd)
	idargs.AddYesFlag(cmd, "Start a manifest-named workload without confirmation.")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"workload_id":        idargs.TelemetryID(ref, args),
			"workload_id_source": ref.Source,
			"output_format":      string(outputFormat),
		}
	})

	return cmd
}
