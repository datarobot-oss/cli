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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/datarobot/cli/cmd/internal/pollflags"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/countflags"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

// followPollInterval matches 'dr workload logs --follow'.
const followPollInterval = 2 * time.Second

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	var (
		limit    int
		level    string
		follow   bool
		interval time.Duration
		grep     []string
		exclude  []string
		since    string
	)

	cmd := &cobra.Command{
		Use:   "logs <job-id> <run-id>",
		Short: "Show a run's logs.",
		Long: `Show what one run of a job wrote to stdout and stderr.

The most recent --limit lines are printed oldest first. Logs outlive the run:
they can be read after it ends and after its pod is gone. Lines reach the log
store a few seconds after they are written, so the last ones of a run that
just ended can take a moment to show up.

--level drops lines below a severity; --grep keeps lines containing the text
(case-insensitive; repeat it to require every term), --exclude drops them;
--since starts from a time (RFC 3339, a date, or a duration back from now:
15m, 2h, 1d). With --follow (-f) new lines stream as they arrive until
Ctrl-C.

Example:
  dr job logs 68b0c1d2e3f4a5b6c7d8e9f0 68b0c1d2e3f4a5b6c7d8e9f1
  dr job logs 68b0c1d2e3f4a5b6c7d8e9f0 68b0c1d2e3f4a5b6c7d8e9f1 --follow
  dr job logs 68b0c1d2e3f4a5b6c7d8e9f0 68b0c1d2e3f4a5b6c7d8e9f1 --level error --limit 500`,
		Args:         cobra.ExactArgs(2),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)
			jobID, runID := args[0], args[1]

			filter, err := buildFilter(runID, level, grep, exclude, since)
			if err != nil {
				return err
			}

			// A job's logs are its workload's; the run is the proton.
			if follow {
				return workload.FollowWorkloadLogs(cmd.Context(), jobID, limit, filter, interval,
					func(e workload.WorkloadLogEntry) error {
						return workload.RenderWorkloadLogLine(outputFormat, e)
					},
					func(msg string) { fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+msg) })
			}

			entries, err := workload.GetWorkloadLogs(jobID, limit, filter)
			if err != nil {
				return err
			}

			if err := workload.RenderWorkloadLogs(outputFormat, entries); err != nil {
				return err
			}

			// stderr, and not under JSON, so stdout stays log lines only.
			if len(entries) == 0 && outputFormat != outputformat.OutputFormatJSON {
				if filter.Narrows() {
					fmt.Fprintln(cmd.ErrOrStderr(), "No logs matched the filters.")
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "No logs for run %s yet. Lines take a few seconds to arrive; "+
						"an empty log can also mean this cluster has no log collection.\n", runID)
				}
			}

			return nil
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().Var(countflags.PositiveInt(&limit, 100), "limit", "Maximum number of recent log lines to return")
	cmd.Flags().StringVar(&level, "level", "", "Minimum log level (debug, info, warn, warning, error, critical)")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Stream new log lines as they arrive (Ctrl-C to stop).")
	cmd.Flags().Var(pollflags.PositiveDuration(&interval, followPollInterval), "poll-interval",
		"Interval between polls when --follow is set.")
	cmd.Flags().StringArrayVar(&grep, "grep", nil,
		"Only lines containing this text, case-insensitive. Repeat to require every term.")
	cmd.Flags().StringArrayVar(&exclude, "exclude", nil,
		"Drop lines containing this text, case-insensitive. Repeatable.")
	cmd.Flags().StringVar(&since, "since", "",
		"Only lines from this time on: RFC 3339, a date, or a duration back from now (15m, 2h, 1d).")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		// Whether each filter was given, never what it said.
		return map[string]any{
			"output_format": string(outputFormat),
			"limit":         limit,
			"level":         level,
			"follow":        follow,
			"grep":          len(grep) > 0,
			"exclude":       len(exclude) > 0,
			"since":         since != "",
		}
	})

	return cmd
}

func buildFilter(runID, level string, grep, exclude []string, since string) (workload.LogFilter, error) {
	parsedLevel, err := workload.ParseLogLevel(level)
	if err != nil {
		return workload.LogFilter{}, err
	}

	for _, term := range append(append([]string{}, grep...), exclude...) {
		if strings.TrimSpace(term) == "" {
			return workload.LogFilter{}, errors.New("an empty search term matches every line; give it text")
		}
	}

	filter := workload.LogFilter{ProtonID: runID, Level: parsedLevel, Grep: grep, Exclude: exclude}

	if since != "" {
		filter.Since, err = workload.ParseLogTime(since, time.Now())
		if err != nil {
			return workload.LogFilter{}, fmt.Errorf("--since: %w", err)
		}
	}

	return filter, nil
}
