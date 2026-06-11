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

	"github.com/datarobot/cli/cmd/workload/internal/pollflags"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

const (
	// A workload settles in minutes (or fails to), so status polls less
	// often and gives up far sooner than the build commands' defaults,
	// which are sized for slow container builds.
	statusPollInterval = 5 * time.Second
	statusPollTimeout  = 5 * time.Minute
)

func Cmd() *cobra.Command {
	var outputFormat workload.OutputFormat

	var poll pollflags.Set

	cmd := &cobra.Command{
		Use:   "status <workload-id>",
		Short: "Show a workload's status.",
		Long: `Show a workload's current status.

Without --wait the status is fetched once and printed as a bare value
(submitted, provisioning, launching, running, suspended, interrupted,
stopping, stopped, errored, terminated, or unknown), making the command
directly usable in scripts. Use 'dr workload get' for the full document.

With --wait the command polls until the workload settles, i.e. leaves
the in-flight states (submitted, provisioning, launching, stopping).
Status transitions are reported on stderr while polling; the final
status is printed on stdout once settled. The command exits non-zero
when the workload settles on "errored" or the poll times out.

JSON output emits one {"id", "status"} document, after settling when
--wait is given.

Example:
  dr workload status 68b0c1d2e3f4a5b6c7d8e9f0
  dr workload status 68b0c1d2e3f4a5b6c7d8e9f0 --wait
  dr workload status 68b0c1d2e3f4a5b6c7d8e9f0 --output-format json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd, args[0], outputFormat, poll)
		},
	}

	workload.AddOutputFlag(cmd, &outputFormat)
	pollflags.RegisterWithDefaults(cmd, &poll, statusPollInterval, statusPollTimeout,
		"Poll until the workload settles.")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"workload_id":   telemetry.FirstArg(args),
			"wait":          poll.Wait,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}

func runStatus(
	cmd *cobra.Command,
	workloadID string,
	outputFormat workload.OutputFormat,
	poll pollflags.Set,
) error {
	if !poll.Wait {
		wl, err := workload.GetWorkload(workloadID)
		if err != nil {
			return err
		}

		// Printing an errored status is a correct answer, not a command
		// failure; only --wait turns an errored settle into a non-zero exit.
		return workload.RenderWorkloadStatus(outputFormat, *wl)
	}

	// All polling progress goes to stderr so stdout stays a single
	// capturable status value.
	fmt.Fprintf(cmd.ErrOrStderr(), "Waiting for workload %s to settle...\n", workloadID)

	var lastStatus string

	wl, waitErr := workload.WaitForWorkloadStatus(workloadID, poll.Interval, poll.Timeout,
		func(w *workload.Workload) {
			if w.Status != lastStatus {
				fmt.Fprintf(cmd.ErrOrStderr(), "  status: %s\n", w.Status)

				lastStatus = w.Status
			}
		})

	if wl == nil {
		return waitErr
	}

	// Only a settled status is rendered: a poll timeout hands back the last
	// in-flight workload, and printing that on stdout would give script
	// captures a non-final value that looks like a final one.
	if workload.IsSettledWorkloadStatus(wl.Status) {
		if err := workload.RenderWorkloadStatus(outputFormat, *wl); err != nil {
			return err
		}
	}

	// An errored settle or poll timeout still fails the command for script
	// callers.
	return waitErr
}
