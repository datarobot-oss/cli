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

package set

import (
	"time"

	"github.com/datarobot/cli/cmd/internal/pollflags"
	"github.com/datarobot/cli/cmd/workload/env/internal/envparse"
	"github.com/datarobot/cli/cmd/workload/env/internal/rollout"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

// replacement settling is closer to a rolling redeploy than a container
// build; these mirror the workload-api skill's bundled
// wait_for_replacement.py defaults (20s / 1200s) rather than pollflags'
// build-oriented defaults.
const (
	defaultPollInterval = 20 * time.Second
	defaultPollTimeout  = 20 * time.Minute
)

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	var poll pollflags.Set

	cmd := &cobra.Command{
		Use:   "set <workload-id> KEY=VALUE [KEY=VALUE ...]",
		Short: "Set one or more environment variables on a workload.",
		Long: `Set environment variables on the artifact a workload is running, then
roll the workload onto the result.

Only the workload's primary container is affected. Artifacts with
additional (sidecar) containers are not yet supported by this command.

Each argument is NAME=VALUE for a plain variable, or
NAME=dr-credential:<credential-id>/<credential-key> to inject a value from a
stored DataRobot credential without ever putting the secret in the artifact
spec:

  - NAME must be a valid environment variable name (letters, digits, '_',
    '-', or '.'; cannot start with a digit) -- checked locally before
    anything is sent, since the platform accepts and silently stores an
    invalid name, only for the container to fail to start much later with
    no obvious link back to this command.
  - <credential-id> is the id of a credential from 'GET /credentials/'
    (a stored secret, e.g. an S3 key pair or an API token). Checked against
    the platform before anything is written, for the same reason.
  - <credential-key> picks which field of that credential to use. A single
    credential can hold several secret fields -- an S3 credential has
    awsAccessKeyId, awsSecretAccessKey, and awsSessionToken, for example --
    so this is a different "key" than the KEY in KEY=VALUE. Not validated:
    if this is wrong, the platform will not catch it either, so a typo here
    still only surfaces once the workload actually tries to run.

    Example: an S3 credential "64f0a1b2c3d4e5f6a7b8c9d0" has two fields you
    might want as separate env vars:
      AWS_ACCESS_KEY_ID=dr-credential:64f0a1b2c3d4e5f6a7b8c9d0/awsAccessKeyId
      AWS_SECRET_ACCESS_KEY=dr-credential:64f0a1b2c3d4e5f6a7b8c9d0/awsSecretAccessKey

Pass every variable you want applied together as one call: each separate
invocation resolves from the workload's currently running artifact, so
multiple calls do not build on each other's staged edits. To load many
variables from a file instead, see 'dr workload env import'.

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
(retry once it settles) -- before parsing arguments or touching any
artifact, so a locked workload with a rollout already underway doesn't get
a wasted throwaway clone created for an edit that couldn't deploy anyway.

Without --stage, this then asks for confirmation (skip with --yes) and
triggers a rolling replacement of the workload onto the resulting artifact.
With --stage, the artifact is prepared but not deployed: no confirmation is
needed, nothing about the running workload changes, and the in-progress-
replacement check above is skipped -- staging never touches the live
rollout machinery, so it's safe even while another replacement settles.

Example:
  dr workload env set 68b0c1d2e3f4a5b6c7d8e9f0 LOG_LEVEL=debug
  dr workload env set 68b0c1d2e3f4a5b6c7d8e9f0 A=1 B=2 --stage
  dr workload env set 68b0c1d2e3f4a5b6c7d8e9f0 API_KEY=dr-credential:64f0.../apiToken --wait`,
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
			"var_count":     len(args) - 1,
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

	stageFlag, _ := cmd.Flags().GetBool("stage")

	// Fail fast, before parsing/validating anything or mutating any
	// artifact, if a rollout couldn't happen right now anyway. Skipped when
	// staging: staging never touches the live rollout machinery, so it's
	// safe to prepare a follow-up edit while an earlier replacement settles.
	if !stageFlag {
		if err := rollout.GuardNoActiveReplacement(workloadID); err != nil {
			return err
		}
	}

	vars := make([]workload.EnvironmentVar, 0, len(args)-1)

	for _, arg := range args[1:] {
		ev, err := envparse.ParseArg(arg)
		if err != nil {
			return err
		}

		vars = append(vars, ev)
	}

	if err := envparse.ValidateCredentialReferences(vars); err != nil {
		return err
	}

	wl, artifact, err := workload.ResolveWorkloadArtifact(workloadID)
	if err != nil {
		return err
	}

	targetArtifactID, needsLock, err := workload.UpsertEnvironmentVars(artifact.ID, vars)
	if err != nil {
		return err
	}

	yesFlag, _ := cmd.Flags().GetBool("yes")

	return rollout.Apply(cmd, format, wl.ID, targetArtifactID, needsLock, rollout.Options{
		Stage: stageFlag,
		Yes:   yesFlag || viperx.GetBool("yes"),
		Poll:  poll,
	})
}
