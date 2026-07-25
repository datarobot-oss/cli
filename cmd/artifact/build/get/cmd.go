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
	"fmt"

	"github.com/datarobot/cli/cmd/artifact/build/internal/buildargs"
	"github.com/datarobot/cli/cmd/internal/pollflags"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	var poll pollflags.Set

	cmd := &cobra.Command{
		Use:   "get [<artifact-id>] <build-id>",
		Short: "Get the status of a single build.",
		Long: `Get a build by id.

When invoked with one positional argument the artifact-id is read from
.wapi/config.json in the current directory. When invoked with two, the
first argument is the artifact-id and the second is the build-id.

With --wait the command polls the build until it reaches a terminal
status (COMPLETED, FAILED, CANCELLED) and prints a summary instead
of the raw Build object. If the build is already terminal, --wait
returns immediately.

Examples:
  dr artifact build get b-xyz-456                # uses .wapi/ artifact id
  dr artifact build get art-abc-123 b-xyz-456
  dr artifact build get art-abc-123 b-xyz-456 --wait
  dr artifact build get art-abc-123 b-xyz-456 --output-format json`,
		Args:         cobra.RangeArgs(1, 2),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			return runGet(cmd, args, outputFormat, poll)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	pollflags.Register(cmd, &poll)

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		artifactID, buildID, _ := buildargs.ResolvePositional(args)

		return map[string]any{
			"artifact_id":   artifactID,
			"build_id":      buildID,
			"wait":          poll.Wait,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}

func runGet(
	cmd *cobra.Command,
	args []string,
	outputFormat outputformat.OutputFormat,
	poll pollflags.Set,
) error {
	artifactID, buildID, err := buildargs.ResolvePositional(args)
	if err != nil {
		return fmt.Errorf("resolve build args: %w", err)
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "Fetching build %s...\n", buildID)

	build, err := workload.GetArtifactBuild(artifactID, buildID)
	if err != nil {
		return fmt.Errorf("get artifact build: %w", err)
	}

	if !poll.Wait {
		if err := workload.RenderBuild(outputFormat, *build); err != nil {
			return fmt.Errorf("render build: %w", err)
		}

		return nil
	}

	return pollAndRender(cmd, artifactID, buildID, build, outputFormat, poll)
}

// pollAndRender implements the --wait branch of runGet: poll the build
// until it reaches a terminal status (or fail), then build a summary
// and render it. Pulled out of runGet to keep runGet's cyclomatic
// complexity under cyclop's threshold.
func pollAndRender(
	cmd *cobra.Command,
	artifactID, buildID string,
	initial *workload.Build,
	outputFormat outputformat.OutputFormat,
	poll pollflags.Set,
) error {
	build, waitErr := waitIfNonTerminal(cmd, artifactID, buildID, initial, outputFormat, poll)

	if build == nil {
		// WaitForBuild errored before its first successful GET; render a
		// minimal summary so the user still sees something.
		summary := workload.BuildSummary{BuildID: buildID, Status: workload.BuildStatusCLIUnknown}
		_ = workload.RenderBuildSummary(outputFormat, summary)

		return fmt.Errorf("wait for build: %w", waitErr)
	}

	if err := renderBuildSummary(outputFormat, build); err != nil {
		return err
	}

	if waitErr != nil {
		return fmt.Errorf("wait for build: %w", waitErr)
	}

	if workload.IsBuildErrorStatus(build.Status) {
		return fmt.Errorf("build %s ended with status %s; run 'dr artifact build logs %s' to inspect", build.ID, build.Status, build.ID)
	}

	return nil
}

// waitIfNonTerminal polls the build until it reaches a terminal status,
// but only if it isn't terminal already. When the build is already
// terminal on first inspection, the original pointer is returned with a
// nil waitErr. When WaitForBuild errored before recording any new
// build, a minimal "unknown" summary is rendered for visibility and
// the wait error is propagated so the caller can wrap it.
func waitIfNonTerminal(
	cmd *cobra.Command,
	artifactID, buildID string,
	build *workload.Build,
	outputFormat outputformat.OutputFormat,
	poll pollflags.Set,
) (*workload.Build, error) {
	if workload.IsTerminalBuildStatus(build.Status) {
		return build, nil
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "Waiting for build %s...\n", buildID)

	polled, waitErr := workload.WaitForBuild(artifactID, buildID, poll.Interval, poll.Timeout, nil)

	if polled == nil {
		return nil, waitErr
	}

	return polled, waitErr
}

// renderBuildSummary builds the CLI summary and writes it. Any
// summary-construction error results in a best-effort partial render
// so the user still gets visible output even when BuildSummaryFor
// fails (e.g. the log-tail endpoint 404s).
func renderBuildSummary(
	outputFormat outputformat.OutputFormat,
	build *workload.Build,
) error {
	summary, serr := workload.BuildSummaryFor(build, workload.DefaultBuildLogTail)
	if serr != nil {
		_ = workload.RenderBuildSummary(outputFormat, summary)

		return fmt.Errorf("build summary: %w", serr)
	}

	if err := workload.RenderBuildSummary(outputFormat, summary); err != nil {
		return fmt.Errorf("render build summary: %w", err)
	}

	return nil
}
