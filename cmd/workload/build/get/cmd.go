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

	"github.com/datarobot/cli/cmd/workload/build/internal/buildargs"
	"github.com/datarobot/cli/cmd/workload/internal/pollflags"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat workload.OutputFormat

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
  dr workload build get b-xyz-456                # uses .wapi/ artifact id
  dr workload build get art-abc-123 b-xyz-456
  dr workload build get art-abc-123 b-xyz-456 --wait
  dr workload build get art-abc-123 b-xyz-456 --output-format json`,
		Args:         cobra.RangeArgs(1, 2),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd, args, outputFormat, poll)
		},
	}

	workload.AddOutputFlag(cmd, &outputFormat)
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
	outputFormat workload.OutputFormat,
	poll pollflags.Set,
) error {
	artifactID, buildID, err := buildargs.ResolvePositional(args)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "Fetching build %s...\n", buildID)

	build, err := workload.GetArtifactBuild(artifactID, buildID)
	if err != nil {
		return err
	}

	if !poll.Wait {
		return workload.RenderBuild(outputFormat, *build)
	}

	var waitErr error

	if !workload.IsTerminalBuildStatus(build.Status) {
		fmt.Fprintf(cmd.ErrOrStderr(), "Waiting for build %s...\n", buildID)

		build, waitErr = workload.WaitForBuild(artifactID, buildID, poll.Interval, poll.Timeout, nil)

		if build == nil {
			// WaitForBuild errored before its first successful GET; render a
			// minimal summary so the user still sees something.
			summary := workload.BuildSummary{BuildID: buildID, Status: workload.BuildStatusCLIUnknown}
			_ = workload.RenderBuildSummary(outputFormat, summary)

			return waitErr
		}
	}

	summary, serr := workload.BuildSummaryFor(build, workload.DefaultBuildLogTail)
	if serr != nil {
		_ = workload.RenderBuildSummary(outputFormat, summary)

		return serr
	}

	if err := workload.RenderBuildSummary(outputFormat, summary); err != nil {
		return err
	}

	if waitErr != nil {
		return waitErr
	}

	// The build may have been already terminal-error on the first GET, in
	// which case WaitForBuild was skipped and waitErr stays nil. Surface
	// that explicitly so the process exits non-zero; hint at the logs
	// command so the user has a one-step recovery to inspect what went
	// wrong.
	if workload.IsBuildErrorStatus(build.Status) {
		return fmt.Errorf("build %s ended with status %s; run 'dr workload build logs %s' to inspect", build.ID, build.Status, build.ID)
	}

	return nil
}
