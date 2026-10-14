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

// Package diagnose implements `dr workload diagnose`: why a workload is in
// the state it is in, read off the platform's per-replica status details.
package diagnose

import (
	"github.com/datarobot/cli/cmd/workload/internal/idargs"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

// diagnoseFn is the read behind the command, a seam so the shell can be
// tested without a platform to ask.
var diagnoseFn = workload.Diagnose

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	var ref idargs.Ref

	cmd := &cobra.Command{
		Use:   "diagnose [<workload-id>]",
		Short: "Explain why a workload is in its current state.",
		Long: `Explain why a workload is in its current state, from the platform's
per-replica status details.

'dr workload status' says "errored" and stops there, and 'dr workload logs'
can be empty for a container that never started, or on a cluster without
log collection. This command reads what the platform records for every
container generation of the workload: the overall verdict, then each
replica and container with its state, reason, restart count, readiness and
how its last run ended. Anything that stands out — CrashLoopBackOff,
ImagePullBackOff, ErrImagePull, OOMKilled, a non-zero exit, restarts — is
listed as a finding, in the words 'dr workload up' uses for the same state.

The generation answering the endpoint is listed first; during a rolling
replacement the one on its way out follows it. A generation the platform's
monitor has not reported for yet is said to have no snapshot.

An errored workload is the answer, not a command failure, so the command
exits zero. Only a read that could not be made is an error.

JSON output is one {"diagnosis": ...} document carrying the platform's own
field names.

` + idargs.HelpText + `

Example:
  dr workload diagnose
  dr workload diagnose 68b0c1d2e3f4a5b6c7d8e9f0
  dr workload diagnose 68b0c1d2e3f4a5b6c7d8e9f0 --output-format json`,
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

			d, err := diagnoseFn(ref.ID)
			if err != nil {
				return ref.Wrap(err)
			}

			return workload.RenderDiagnosisTo(cmd.OutOrStdout(), outputFormat, *d)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	idargs.AddDirFlag(cmd)

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"workload_id":        idargs.TelemetryID(ref, args),
			"workload_id_source": ref.Source,
			"output_format":      string(outputFormat),
		}
	})

	return cmd
}
