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

package events

import (
	"fmt"
	"time"

	"github.com/datarobot/cli/cmd/workload/internal/idargs"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/countflags"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

// listEventsFn is the client call, a variable so tests can stand in for it.
var listEventsFn = workload.ListWorkloadEvents

// filterFlags are the --type/--since/--until/--proton-id values as typed.
type filterFlags struct {
	types    []string
	since    string
	until    string
	protonID string
}

// build turns the typed values into a filter, reading the window against
// now so "2h" means the same thing on both sides.
func (f filterFlags) build(now time.Time) (workload.EventFilter, error) {
	filter := workload.EventFilter{Types: f.types, ProtonID: f.protonID}

	var err error

	if f.since != "" {
		if filter.Since, err = workload.ParseLogTime(f.since, now); err != nil {
			return filter, fmt.Errorf("--since: %w", err)
		}
	}

	if f.until != "" {
		if filter.Until, err = workload.ParseLogUntil(f.until, now); err != nil {
			return filter, fmt.Errorf("--until: %w", err)
		}
	}

	if !filter.Since.IsZero() && !filter.Until.IsZero() && filter.Until.Before(filter.Since) {
		return filter, fmt.Errorf("--until (%s) is before --since (%s)",
			filter.Until.UTC().Format(time.RFC3339), filter.Since.UTC().Format(time.RFC3339))
	}

	return filter, nil
}

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	var ref idargs.Ref

	var (
		limit   int
		filters filterFlags
	)

	cmd := &cobra.Command{
		Use:   "events [<workload-id>]",
		Short: "Show a workload's lifecycle events.",
		Long: `Show the lifecycle events the platform recorded for a workload: what
changed, when, and who did it. A settings change that rolled out, a
rollout that failed and why, a start or a stop.

The most recent --limit events are printed oldest first. Narrow them with
--type (a substring of the event type, any case, repeatable), --since and
--until (an RFC 3339 time, a date, or a duration back from now such as
2h or 1d; a date given to --until covers the whole of that day), and
--proton-id (events that name the given generation).

By default, output is a table with the platform's own message for each
event. Use --output-format json for the complete records, details
included, as one {"events": [...]} document.

` + idargs.HelpText + `

Example:
  dr workload events
  dr workload events 68b0c1d2e3f4a5b6c7d8e9f0
  dr workload events 68b0c1d2e3f4a5b6c7d8e9f0 --type errored
  dr workload events 68b0c1d2e3f4a5b6c7d8e9f0 --since 1d --until 2h
  dr workload events 68b0c1d2e3f4a5b6c7d8e9f0 --output-format json`,
		Args:         cobra.MaximumNArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			filter, err := filters.build(time.Now())
			if err != nil {
				return err
			}

			// After the flag checks, so a bad flag is not reported as a
			// missing manifest, and before anything reaches the network.
			ref, err = idargs.Resolve(cmd, args)
			if err != nil {
				return err
			}

			events, err := listEventsFn(ref.ID, limit, filter)
			if err != nil {
				return ref.Wrap(err)
			}

			return workload.RenderWorkloadEventsTo(cmd.OutOrStdout(), outputFormat, events)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	idargs.AddDirFlag(cmd)

	cmd.Flags().Var(countflags.PositiveInt(&limit, 100), "limit", "Maximum number of recent events to show")
	cmd.Flags().StringArrayVar(&filters.types, "type", nil,
		"Only events whose type contains this text, any case (repeatable, any of them)")
	cmd.Flags().StringVar(&filters.since, "since", "", "Only events at or after this time (RFC 3339, a date, or 15m/2h/1d/1w back)")
	cmd.Flags().StringVar(&filters.until, "until", "",
		"Only events at or before this time (same forms as --since; a date covers the whole day)")
	cmd.Flags().StringVar(&filters.protonID, "proton-id", "", "Only events that name this generation (proton) id")

	telemetry.TrackWith(cmd, func(c *cobra.Command, args []string) map[string]any {
		limit, _ := c.Flags().GetInt("limit")

		return map[string]any{
			"workload_id":        idargs.TelemetryID(ref, args),
			"workload_id_source": ref.Source,
			"limit":              limit,
			"types":              len(filters.types),
			"since":              filters.since != "",
			"until":              filters.until != "",
			"proton_id":          filters.protonID != "",
			"output_format":      string(outputFormat),
		}
	})

	return cmd
}
