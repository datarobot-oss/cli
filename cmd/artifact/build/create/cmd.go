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

package create

import (
	"errors"
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
		Use:   "create [<artifact-id>]",
		Short: "Trigger a new image build for an artifact.",
		Long: `Trigger a new container image build for an artifact.

The artifact-id argument is optional when run inside a directory linked
via 'dr artifact code init': the id is read from .wapi/config.json.

By default the command prints the new build IDs (one per line) and
exits. With --wait it polls each build until it reaches a terminal
status (COMPLETED, FAILED, or CANCELLED) and prints a summary with
duration and resulting image_uri. On failure the tail of the build
logs is dumped to stderr.

JSON output emits one document:
  - without --wait: the raw trigger response {"buildIds":[...]}.
  - with --wait: an array of BuildSummary objects, always an array.

Examples:
  dr artifact build create
  dr artifact build create art-abc-123
  dr artifact build create art-abc-123 --wait
  dr artifact build create --output-format json | jq .buildIds`,
		Args:         cobra.MaximumNArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			return runTrigger(cmd, args, outputFormat, poll)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	pollflags.Register(cmd, &poll)

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"artifact_id":   telemetry.FirstArg(args),
			"wait":          poll.Wait,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}

func runTrigger(
	cmd *cobra.Command,
	args []string,
	outputFormat outputformat.OutputFormat,
	poll pollflags.Set,
) error {
	artifactID, err := buildargs.ResolveOptional(args)
	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.ErrOrStderr(), "Triggering build...")

	resp, err := workload.TriggerArtifactBuild(artifactID)
	if err != nil {
		return err
	}

	if len(resp.BuildIDs) == 0 {
		return errors.New("no build IDs returned by server")
	}

	if !poll.Wait {
		// Script-capture contract: text-mode `BID=$(dr artifact build
		// create $ART)` gets just the IDs on stdout. JSON callers parse
		// the trigger response document.
		return workload.RenderBuildTrigger(outputFormat, *resp)
	}

	// With --wait the canonical stdout contract is the BuildSummary(ies)
	// emitted after polling. Print the loose IDs to stderr so Ctrl-C
	// users keep the handle (per RAPTOR-17387) but the captured stdout
	// stream stays uncontaminated and `jq` works in JSON mode.
	if outputFormat == outputformat.OutputFormatText {
		for _, id := range resp.BuildIDs {
			fmt.Fprintln(cmd.ErrOrStderr(), id)
		}
	}

	summaries, firstWaitErr := waitForAllBuilds(cmd, artifactID, resp.BuildIDs, poll)

	if err := workload.RenderBuildSummaries(outputFormat, summaries); err != nil {
		return err
	}

	return firstWaitErr
}

func waitForAllBuilds(
	cmd *cobra.Command,
	artifactID string,
	buildIDs []string,
	poll pollflags.Set,
) ([]workload.BuildSummary, error) {
	summaries := make([]workload.BuildSummary, 0, len(buildIDs))

	var firstWaitErr error

	for _, buildID := range buildIDs {
		fmt.Fprintf(cmd.ErrOrStderr(), "Waiting for build %s...\n", buildID)

		build, werr := workload.WaitForBuild(artifactID, buildID, poll.Interval, poll.Timeout, nil)
		if werr != nil && firstWaitErr == nil {
			firstWaitErr = werr
		}

		if build == nil {
			summaries = append(summaries, workload.BuildSummary{BuildID: buildID, Status: workload.BuildStatusCLIUnknown})

			continue
		}

		summary, serr := workload.BuildSummaryFor(build, workload.DefaultBuildLogTail)
		if serr != nil && firstWaitErr == nil {
			firstWaitErr = serr
		}

		summaries = append(summaries, summary)
	}

	return summaries, firstWaitErr
}
