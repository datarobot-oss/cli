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

// Package rollout holds the deploy-or-stage decision shared by
// `dr workload env set` and `dr workload env delete` once each has computed
// the artifact id it wants the workload running.
package rollout

import (
	"errors"
	"fmt"

	"github.com/datarobot/cli/cmd/helpers"
	"github.com/datarobot/cli/cmd/internal/pollflags"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

// Options bundles the flag-derived inputs Apply needs from either caller.
type Options struct {
	Stage bool
	Yes   bool
	Poll  pollflags.Set
}

// Apply resolves what happens to targetArtifactID once an env var edit has
// produced it against workloadID's currently-running artifact.
//
//   - With Stage: report the artifact id and stop. No lock, no confirmation,
//     no rollout -- the artifact stays a deletable, re-editable draft.
//     needsLock is intentionally ignored here (see ApplyEnvironmentVars's
//     doc for why staged locking would leave permanent orphaned artifacts).
//   - Otherwise: confirm (unless Yes) that this is a rolling redeploy, lock
//     targetArtifactID first if needsLock (it must match the running
//     artifact's locked status before a replacement can start), guard
//     against an already-in-flight replacement, start one, and render or
//     wait for the outcome per Poll.
func Apply(
	cmd *cobra.Command,
	format outputformat.OutputFormat,
	workloadID, targetArtifactID string,
	needsLock bool,
	opts Options,
) error {
	if opts.Stage {
		printStaged(cmd, targetArtifactID)

		return nil
	}

	confirmed, err := confirmRollout(cmd, opts.Yes, workloadID, targetArtifactID)
	if err != nil || !confirmed {
		return err
	}

	if needsLock {
		if _, err := workload.LockArtifact(targetArtifactID); err != nil {
			return fmt.Errorf("lock artifact %s before rollout: %w", targetArtifactID, err)
		}
	}

	if err := GuardNoActiveReplacement(workloadID); err != nil {
		return err
	}

	if _, err := workload.StartReplacement(workloadID, targetArtifactID); err != nil {
		// The env var edit already landed on targetArtifactID (locked, if
		// needsLock was set) even though the rollout itself failed -- name
		// it so the caller can retry the replacement (once the underlying
		// problem, e.g. a limit or a status mismatch, is fixed) instead of
		// starting the whole edit over.
		return fmt.Errorf("start replacement of workload %s onto prepared artifact %s: %w", workloadID, targetArtifactID, err)
	}

	if !opts.Poll.Wait {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"Replacement started: workload %s -> artifact %s. Check progress with 'dr workload get %s', or re-run with --wait.\n",
			workloadID, targetArtifactID, workloadID)

		return nil
	}

	return waitAndRender(cmd, format, workloadID, opts.Poll)
}

// confirmRollout asks for confirmation before a rolling redeploy, unless
// yes bypasses it. Returns (false, nil) on a declined prompt so the caller
// exits cleanly rather than treating "no" as an error.
func confirmRollout(cmd *cobra.Command, yes bool, workloadID, targetArtifactID string) (bool, error) {
	if yes {
		return true, nil
	}

	if !reader.IsStdinTerminal() {
		return false, errors.New("confirmation required: pass --yes (or set DATAROBOT_CLI_NON_INTERACTIVE=1) to roll out without a prompt")
	}

	confirmed, err := helpers.Confirm(cmd.OutOrStdout(), cmd.InOrStdin(),
		fmt.Sprintf("This triggers a rolling redeploy of workload %s onto artifact %s. Continue? [y/N] ",
			workloadID, targetArtifactID))
	if err != nil {
		return false, err
	}

	if !confirmed {
		fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
	}

	return confirmed, nil
}

// GuardNoActiveReplacement errors out if workloadID already has a
// replacement in flight -- POSTing another would queue a second swap rather
// than reject, since the endpoint is not idempotent. Exported so `env
// set`/`env delete` can call it as an upfront fail-fast check, before
// parsing/validating arguments or mutating any artifact, in addition to
// Apply's own call immediately before StartReplacement (which remains the
// actual correctness guard -- state can still change in the time it takes
// to get from here to there).
func GuardNoActiveReplacement(workloadID string) error {
	active, err := workload.GetActiveReplacement(workloadID)
	if err != nil {
		return err
	}

	if active == nil {
		return nil
	}

	return fmt.Errorf(
		"workload %s already has a replacement in progress (to artifact %s, status %s); wait for it to settle before starting another",
		workloadID, active.ArtifactID, active.Status,
	)
}

func waitAndRender(cmd *cobra.Command, format outputformat.OutputFormat, workloadID string, poll pollflags.Set) error {
	fmt.Fprintf(cmd.ErrOrStderr(), "Waiting for replacement on workload %s...\n", workloadID)

	final, waitErr := workload.WaitForReplacement(workloadID, poll.Interval, poll.Timeout)
	if final == nil {
		if waitErr != nil {
			return waitErr
		}

		return errors.New("replacement wait returned no result")
	}

	if renderErr := workload.RenderReplacement(format, *final); renderErr != nil {
		return renderErr
	}

	return waitErr
}

func printStaged(cmd *cobra.Command, targetArtifactID string) {
	fmt.Fprintf(cmd.OutOrStdout(), "Staged artifact %s (not deployed).\n", targetArtifactID)
	fmt.Fprintln(cmd.ErrOrStderr(),
		"This edit is not live yet. A later 'env set'/'env delete' call starts over from the "+
			"workload's currently running artifact, not this staged one -- include every var you want "+
			"batched together in a single command. Deploy this exact edit by re-running the same "+
			"command without --stage.")
}
