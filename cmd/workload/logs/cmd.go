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

package logs

import (
	"fmt"
	"time"

	"github.com/datarobot/cli/cmd/workload/internal/pollflags"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

// followPollInterval is the default cadence at which --follow polls for new
// log lines. Hidden behind --poll-interval for tuning.
const followPollInterval = 2 * time.Second

func Cmd() *cobra.Command {
	var outputFormat workload.OutputFormat

	var (
		limit    int
		level    string
		follow   bool
		interval time.Duration
	)

	cmd := &cobra.Command{
		Use:   "logs <workload-id>",
		Short: "Show a workload's container logs.",
		Long: `Show the application logs from a workload's running container(s).

By default the most recent --limit lines are printed in chronological
order (oldest first), like 'kubectl logs --tail'. Use --level to drop
everything below a severity (debug, info, warn, warning, error,
critical); debug (the default) keeps every line.

With --follow (-f) the command keeps running and streams new log lines as
they arrive (like 'tail -f'), starting from the most recent --limit lines.
Press Ctrl-C to stop.

By default, output is a human-readable "[LEVEL] timestamp message" line
per entry. Use --output-format json for machine-parseable output: a JSON
array without --follow, or one JSON object per line (JSON Lines) with
--follow.

Example:
  dr workload logs 68b0c1d2e3f4a5b6c7d8e9f0
  dr workload logs 68b0c1d2e3f4a5b6c7d8e9f0 --limit 500
  dr workload logs 68b0c1d2e3f4a5b6c7d8e9f0 --level error
  dr workload logs 68b0c1d2e3f4a5b6c7d8e9f0 --follow
  dr workload logs 68b0c1d2e3f4a5b6c7d8e9f0 --output-format json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit <= 0 {
				return fmt.Errorf("invalid --limit %d: must be positive", limit)
			}

			parsedLevel, err := workload.ParseLogLevel(level)
			if err != nil {
				return err
			}

			if follow {
				// Warnings go to stderr so stdout stays log lines only.
				onWarn := func(msg string) {
					fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+msg)
				}

				return workload.FollowWorkloadLogs(cmd.Context(), args[0], limit, parsedLevel, interval,
					func(e workload.WorkloadLogEntry) error {
						return workload.RenderWorkloadLogLine(outputFormat, e)
					}, onWarn)
			}

			entries, err := workload.GetWorkloadLogs(args[0], limit, parsedLevel)
			if err != nil {
				return err
			}

			return workload.RenderWorkloadLogs(outputFormat, entries)
		},
	}

	workload.AddOutputFlag(cmd, &outputFormat)
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum number of recent log lines to return")
	cmd.Flags().StringVar(&level, "level", "", "Minimum log level (debug, info, warn, warning, error, critical)")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Stream new log lines as they arrive (Ctrl-C to stop).")
	cmd.Flags().Var(pollflags.PositiveDuration(&interval, followPollInterval), "poll-interval",
		"Interval between polls when --follow is set.")
	_ = cmd.Flags().MarkHidden("poll-interval")

	telemetry.TrackWith(cmd, func(c *cobra.Command, args []string) map[string]any {
		limit, _ := c.Flags().GetInt("limit")

		return map[string]any{
			"workload_id":   telemetry.FirstArg(args),
			"limit":         limit,
			"level":         level,
			"follow":        follow,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
