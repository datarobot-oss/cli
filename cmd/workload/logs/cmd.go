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
	"io"
	"strings"
	"time"

	"github.com/datarobot/cli/cmd/internal/pollflags"
	"github.com/datarobot/cli/cmd/workload/internal/idargs"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/countflags"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

// followPollInterval is the default cadence at which --follow polls for new
// log lines; --poll-interval overrides it.
const followPollInterval = 2 * time.Second

// renderOnce prints a single fetch and, on stderr, what an empty one means.
//
// An empty result under a filter is not "the workload has no logs", which is
// what the plain wording (and the docs note it points at) would say; JSON
// still gets its [] on stdout. An empty unfiltered answer is not a dead end
// either: a container that never started wrote nothing, and a cluster
// without log collection answers 200 with nothing for a healthy workload
// too; the per-replica status details are where the reason lives. Both notes
// go to stderr so stdout stays log lines only, and neither under JSON, where
// stderr is kept clear for `2>&1 | jq .`.
func renderOnce(stderr io.Writer, format outputformat.OutputFormat, ref idargs.Ref,
	filter workload.LogFilter, entries []workload.WorkloadLogEntry,
) error {
	if len(entries) == 0 && filter.Narrows() && format != outputformat.OutputFormatJSON {
		fmt.Fprintln(stderr, "No logs matched the filters.")

		return nil
	}

	if err := workload.RenderWorkloadLogs(format, entries); err != nil {
		return err
	}

	if len(entries) == 0 && format != outputformat.OutputFormatJSON {
		fmt.Fprintf(stderr,
			"Run 'dr workload diagnose %s' to see why the containers are not running; "+
				"an empty log can also mean this cluster has no log collection.\n", ref.ID)
	}

	return nil
}

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	var ref idargs.Ref

	var (
		limit    int
		level    string
		follow   bool
		interval time.Duration
		f        filterFlags
	)

	cmd := &cobra.Command{
		Use:   "logs [<workload-id>]",
		Short: "Show a workload's container logs.",
		Long: `Show the application logs from a workload's running container(s).

By default the most recent --limit lines are printed in chronological
order (oldest first), like 'kubectl logs --tail'. Use --level to drop
everything below a severity (debug, info, warn, warning, error,
critical); debug (the default) keeps every line.

Narrow further with --grep (lines containing the text, case-insensitive;
repeat it to require every term), --exclude (drop lines containing the
text; repeatable), --trace-id and --span-id, and a time window from --since
to --until. A time is RFC 3339 (2026-06-11T14:04:15Z), a date, or a duration
back from now: 15m, 2h30m, 1d, 1w. A time without a zone is UTC, and a date
given to --until covers the whole of that day. --limit bounds what is fetched, before
--exclude and any second --grep term are applied, so a filtered result can
be shorter than the limit.

With --follow (-f) the command keeps running and streams new log lines as
they arrive (like 'tail -f'), starting from the most recent --limit lines.
--since narrows that first batch; --until has no meaning for a stream that
has no end and is refused. Press Ctrl-C to stop.

By default, output is a human-readable "[LEVEL] timestamp message" line
per entry. Use --output-format json for machine-parseable output: a JSON
array without --follow, or one JSON object per line (JSON Lines) with
--follow.

` + idargs.HelpText + `

Example:
  dr workload logs
  dr workload logs 68b0c1d2e3f4a5b6c7d8e9f0
  dr workload logs 68b0c1d2e3f4a5b6c7d8e9f0 --limit 500
  dr workload logs 68b0c1d2e3f4a5b6c7d8e9f0 --level error
  dr workload logs 68b0c1d2e3f4a5b6c7d8e9f0 --grep "connection refused" --since 1h
  dr workload logs 68b0c1d2e3f4a5b6c7d8e9f0 --exclude healthz --follow
  dr workload logs 68b0c1d2e3f4a5b6c7d8e9f0 --trace-id 4bf92f3577b34da6a3ce929d0e0e4736
  dr workload logs 68b0c1d2e3f4a5b6c7d8e9f0 --output-format json`,
		Args:         cobra.MaximumNArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			parsedLevel, err := workload.ParseLogLevel(level)
			if err != nil {
				return err
			}

			filter, err := f.build(parsedLevel, follow, time.Now())
			if err != nil {
				return err
			}

			// After the flag checks, so a bad --limit is not reported as a
			// missing manifest, and before anything reaches the network.
			ref, err = idargs.Resolve(cmd, args)
			if err != nil {
				return err
			}

			if follow {
				// Warnings go to stderr so stdout stays log lines only.
				onWarn := func(msg string) {
					fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+msg)
				}

				return ref.Wrap(workload.FollowWorkloadLogs(cmd.Context(), ref.ID, limit, filter, interval,
					func(e workload.WorkloadLogEntry) error {
						return workload.RenderWorkloadLogLine(outputFormat, e)
					}, onWarn))
			}

			entries, err := workload.GetWorkloadLogs(ref.ID, limit, filter)
			if err != nil {
				return ref.Wrap(err)
			}

			return renderOnce(cmd.ErrOrStderr(), outputFormat, ref, filter, entries)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	idargs.AddDirFlag(cmd)

	cmd.Flags().Var(countflags.PositiveInt(&limit, 100), "limit", "Maximum number of recent log lines to return")
	cmd.Flags().StringVar(&level, "level", "", "Minimum log level (debug, info, warn, warning, error, critical)")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Stream new log lines as they arrive (Ctrl-C to stop).")
	cmd.Flags().Var(pollflags.PositiveDuration(&interval, followPollInterval), "poll-interval",
		"Interval between polls when --follow is set.")

	cmd.Flags().StringArrayVar(&f.grep, "grep", nil,
		"Only lines containing this text, case-insensitive. Repeat to require every term.")
	cmd.Flags().StringArrayVar(&f.exclude, "exclude", nil,
		"Drop lines containing this text, case-insensitive. Repeatable.")
	cmd.Flags().StringVar(&f.traceID, "trace-id", "", "Only lines from this trace.")
	cmd.Flags().StringVar(&f.spanID, "span-id", "", "Only lines from this span.")
	cmd.Flags().StringVar(&f.since, "since", "",
		"Only lines from this time on: RFC 3339, a date, or a duration back from now (15m, 2h, 1d, 1w). Times without a zone are UTC.")
	cmd.Flags().StringVar(&f.until, "until", "",
		"Only lines up to this time, in the same forms as --since; a date covers the whole day. Not with --follow.")

	telemetry.TrackWith(cmd, func(c *cobra.Command, args []string) map[string]any {
		limit, _ := c.Flags().GetInt("limit")

		// Whether each filter was given, never what it said: a search term
		// or a trace id is the user's data.
		return map[string]any{
			"workload_id":        idargs.TelemetryID(ref, args),
			"workload_id_source": ref.Source,
			"limit":              limit,
			"level":              level,
			"follow":             follow,
			"grep":               len(f.grep) > 0,
			"exclude":            len(f.exclude) > 0,
			"trace_id":           f.traceID != "",
			"span_id":            f.spanID != "",
			"since":              f.since != "",
			"until":              f.until != "",
			"output_format":      string(outputFormat),
		}
	})

	return cmd
}

// filterFlags is the narrowing the command line asked for, before it is
// checked and turned into the filter the client applies.
type filterFlags struct {
	grep, exclude   []string
	traceID, spanID string
	since, until    string
}

// build checks the flags against each other and reads the times, naming the
// flag at fault: the client package has no flags to name.
func (f filterFlags) build(level string, follow bool, now time.Time) (workload.LogFilter, error) {
	if err := f.checkTerms(); err != nil {
		return workload.LogFilter{}, err
	}

	since, until, err := f.window(follow, now)
	if err != nil {
		return workload.LogFilter{}, err
	}

	return workload.LogFilter{
		Level:   level,
		Grep:    f.grep,
		Exclude: f.exclude,
		TraceID: f.traceID,
		SpanID:  f.spanID,
		Since:   since,
		Until:   until,
	}, nil
}

// checkTerms refuses an empty search term: it matches every line, which is
// never what was meant, and would quietly turn a filter into no filter.
func (f filterFlags) checkTerms() error {
	for _, flag := range []struct {
		name  string
		terms []string
	}{{"--grep", f.grep}, {"--exclude", f.exclude}} {
		for _, term := range flag.terms {
			if strings.TrimSpace(term) == "" {
				return fmt.Errorf("%s: an empty search term matches every line; give it text", flag.name)
			}
		}
	}

	return nil
}

// window reads --since and --until, and refuses the combinations that
// cannot mean anything: an end on a follow, or a start after the end.
func (f filterFlags) window(follow bool, now time.Time) (since, until time.Time, err error) {
	if f.since != "" {
		if since, err = workload.ParseLogTime(f.since, now); err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("--since: %w", err)
		}
	}

	if f.until != "" {
		if follow {
			return time.Time{}, time.Time{}, errors.New("--until cannot be combined with --follow: a follow has no end")
		}

		if until, err = workload.ParseLogUntil(f.until, now); err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("--until: %w", err)
		}
	}

	if !since.IsZero() && !until.IsZero() && since.After(until) {
		return time.Time{}, time.Time{}, fmt.Errorf("--since (%s) is after --until (%s), so the window is empty",
			since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339))
	}

	return since, until, nil
}
