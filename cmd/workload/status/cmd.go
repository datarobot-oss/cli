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
	"fmt"
	"time"

	"github.com/datarobot/cli/cmd/internal/pollflags"
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

	var (
		watch    bool
		interval time.Duration
		timeout  time.Duration
	)

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

--watch follows the workload until it settles (running, stopped, suspended,
errored or terminated), printing each status as it changes, then exits with
the last one; Ctrl-C stops watching, not the workload. With
--output-format json only the settled status is printed.

` + idargs.HelpText + `

Example:
  dr workload status
  dr workload status 68b0c1d2e3f4a5b6c7d8e9f0
  dr workload status 68b0c1d2e3f4a5b6c7d8e9f0 --output-format json
  dr workload status 68b0c1d2e3f4a5b6c7d8e9f0 --watch`,
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

			if watch {
				return watchStatus(cmd, ref, outputFormat, interval, timeout)
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
	cmd.Flags().BoolVar(&watch, "watch", false, "Follow the status until the workload settles.")
	cmd.Flags().Var(pollflags.PositiveDuration(&interval, watchInterval), "poll-interval", "Interval between status polls with --watch.")
	cmd.Flags().Var(pollflags.PositiveDuration(&timeout, pollflags.DefaultPollTimeout), "poll-timeout",
		"Maximum time to watch. Giving up ends the watch, not what it is watching.")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"workload_id":        idargs.TelemetryID(ref, args),
			"workload_id_source": ref.Source,
			"output_format":      string(outputFormat),
			"watch":              watch,
		}
	})

	return cmd
}

// watchInterval is the default poll interval for --watch: a workload moves on
// the scale of seconds, and a watch is something a person looks at.
const watchInterval = 5 * time.Second

// watchStatus polls until the workload settles. In text mode each new status
// is printed once as it is seen; the settled one is the last line. An errored
// workload is an answer, not a failure, exactly as without --watch.
func watchStatus(cmd *cobra.Command, ref idargs.Ref, format outputformat.OutputFormat, interval, timeout time.Duration) error {
	last := ""

	onTick := func(wl *workload.Workload) {
		if format == outputformat.OutputFormatJSON || wl.Status == last {
			return
		}

		last = wl.Status

		fmt.Fprintln(cmd.OutOrStdout(), wl.Status)
	}

	wl, err := workload.WaitForSteadyWorkload(cmd.Context(), ref.ID, interval, timeout, onTick)
	if err != nil {
		return ref.Wrap(err)
	}

	if format == outputformat.OutputFormatJSON {
		return workload.RenderWorkloadStatus(format, *wl)
	}

	if wl.Status != last {
		fmt.Fprintln(cmd.OutOrStdout(), wl.Status)
	}

	return nil
}
