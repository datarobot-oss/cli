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

// Package del implements the `dr workload env delete` verb. The directory is
// named `del` rather than `delete` because the latter shadows Go's built-in
// delete() function in importing files (same rationale as cmd/workload/del).
package del

import (
	"errors"
	"strings"
	"time"

	"github.com/datarobot/cli/cmd/internal/pollflags"
	"github.com/datarobot/cli/cmd/workload/env/internal/rollout"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

// Mirrors cmd/workload/env/set's poll defaults -- replacement settling is
// closer to a rolling redeploy than a container build.
const (
	defaultPollInterval = 20 * time.Second
	defaultPollTimeout  = 20 * time.Minute
)

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	var poll pollflags.Set

	cmd := &cobra.Command{
		Use:   "delete <workload-id> KEY [KEY ...]",
		Short: "Remove one or more environment variables from a workload.",
		Long: `Remove environment variables from the artifact a workload is running,
then roll the workload onto the result.

Only the workload's primary container is affected. Artifacts with
additional (sidecar) containers are not yet supported by this command.

Pass every name you want removed together as one call: each separate
invocation resolves from the workload's currently running artifact, so
multiple calls do not build on each other's staged edits.

Concurrent edits to the SAME workload can silently clobber each other: this
command reads the current spec, merges your change, and writes the whole
spec back, with no conflict detection. Avoid running 'env set'/'env delete'
against the same workload from two sessions at once -- whichever write
lands last wins, and the other's change is lost without any error.

If the workload's current artifact is a draft, the change is applied to it
in place. If it is locked (locking is one-way and irreversible), a new
artifact is cloned from it and edited instead -- the workload itself is not
touched until the rollout below runs.

Unless --stage is given, this first checks that the workload doesn't
already have a replacement in progress and refuses to proceed if it does
(retry once it settles) -- before resolving or mutating any artifact, so a
locked workload with a rollout already underway doesn't get a wasted
throwaway clone created for an edit that couldn't deploy anyway.

Without --stage, this then asks for confirmation (skip with --yes) and
triggers a rolling replacement of the workload onto the resulting artifact.
With --stage, the artifact is prepared but not deployed: no confirmation is
needed, nothing about the running workload changes, and the in-progress-
replacement check above is skipped -- staging never touches the live
rollout machinery, so it's safe even while another replacement settles.

Removing a name that isn't currently set is a no-op for that name; the
command errors only if none of the given names were present.

Example:
  dr workload env delete 68b0c1d2e3f4a5b6c7d8e9f0 LOG_LEVEL
  dr workload env delete 68b0c1d2e3f4a5b6c7d8e9f0 A B --yes --wait`,
		Args:         cobra.MinimumNArgs(2),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			return run(cmd, outputFormat, args, poll)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	pollflags.RegisterWithDefaults(cmd, &poll, defaultPollInterval, defaultPollTimeout, "Poll until the replacement settles.")

	cmd.Flags().Bool("stage", false, "Apply the edit without triggering a rollout.")
	cmd.Flags().BoolP("yes", "y", false, "Skip the rollout confirmation prompt.")

	_ = viperx.BindEnv("yes", "DATAROBOT_CLI_NON_INTERACTIVE")

	telemetry.TrackWith(cmd, func(cmd *cobra.Command, args []string) map[string]any {
		stageFlag, _ := cmd.Flags().GetBool("stage")
		yesFlag, _ := cmd.Flags().GetBool("yes")

		return map[string]any{
			"workload_id":   telemetry.FirstArg(args),
			"name_count":    len(args) - 1,
			"stage":         stageFlag,
			"yes":           yesFlag || viperx.GetBool("yes"),
			"wait":          poll.Wait,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}

func run(cmd *cobra.Command, format outputformat.OutputFormat, args []string, poll pollflags.Set) error {
	workloadID := args[0]
	names := args[1:]

	stageFlag, _ := cmd.Flags().GetBool("stage")

	// Fail fast, before resolving/mutating any artifact, if a rollout
	// couldn't happen right now anyway. Skipped when staging: staging never
	// touches the live rollout machinery, so it's safe to prepare a
	// follow-up edit while an earlier replacement settles.
	if !stageFlag {
		if err := rollout.GuardNoActiveReplacement(workloadID); err != nil {
			return err
		}
	}

	wl, artifact, err := workload.ResolveWorkloadArtifact(workloadID)
	if err != nil {
		return err
	}

	// Bail before touching the API at all if none of the names are actually
	// set: if the current artifact is locked, RemoveEnvironmentVars would
	// otherwise clone it (a throwaway, unlocked draft -- harmless, but
	// pointless litter) only to discover afterward that nothing changed.
	if present := workload.PresentEnvironmentVarNames(workload.PrimaryEnvironmentVars(*artifact), names); len(present) == 0 {
		return errors.New("none of the given names were set on workload " + workloadID +
			"'s current artifact: " + strings.Join(names, ", "))
	}

	targetArtifactID, needsLock, removed, err := workload.RemoveEnvironmentVars(artifact.ID, names)
	if err != nil {
		return err
	}

	if len(removed) == 0 {
		// Should be unreachable given the pre-check above, short of a
		// concurrent edit racing between the read and the write.
		return errors.New("none of the given names were set on workload " + workloadID +
			"'s current artifact: " + strings.Join(names, ", "))
	}

	yesFlag, _ := cmd.Flags().GetBool("yes")

	return rollout.Apply(cmd, format, wl.ID, targetArtifactID, needsLock, rollout.Options{
		Stage: stageFlag,
		Yes:   yesFlag || viperx.GetBool("yes"),
		Poll:  poll,
	})
}
