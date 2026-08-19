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

// Package del implements the `dr workload delete` verb. The directory is
// named `del` rather than `delete` because the latter shadows Go's built-in
// delete() function in importing files.
package del

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"

	"github.com/datarobot/cli/cmd/helpers"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/fsutil"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <workload-id>",
		Short: "Delete a workload.",
		Long: `Delete a workload by id.

Deleting a running workload is allowed: the platform stops the backing
replicas first, then removes the workload. The artifact it was created from
is not deleted with it; remove that separately with
'dr artifact delete <artifact-id>' once no workload references it.

If the .datarobot.yaml found from --dir (the current directory by default,
searched upward from there) is bound to the workload being deleted, its
workloadId is removed too, so the next 'dr workload up' creates a new
workload rather than pointing at one that is gone. Only a manifest naming
that exact id is touched. Pass the same --dir you deployed with: a manifest
in a subdirectory is not visible from its parent.

Without --yes the command asks for confirmation.

Example:
  dr workload delete 68b0c1d2e3f4a5b6c7d8e9f0
  dr workload delete 68b0c1d2e3f4a5b6c7d8e9f0 --yes`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			confirmed, err := confirmDelete(cmd, args[0])
			if err != nil || !confirmed {
				return err
			}

			// Not discarded: a lookup failure means the flag was renamed or
			// dropped, and the silent fallback is a search from the working
			// directory, which is the behaviour --dir exists to replace.
			dir, err := cmd.Flags().GetString("dir")
			if err != nil {
				return fmt.Errorf("cannot read --dir: %w", err)
			}

			// Checked before the delete, not after: a typo is worth catching
			// while it still costs nothing, and the cleanup that reports it
			// runs on the far side of an irreversible call.
			if dir != "" && !fsutil.DirExists(dir) {
				return fmt.Errorf("--dir %s: %w", dir, manifest.ErrNotADirectory)
			}

			if err := workload.DeleteWorkload(args[0]); err != nil {
				return handleDeleteError(err, args[0])
			}

			fmt.Println(tui.BaseTextStyle.Render("Deleted workload: " + args[0]))

			clearStaleBinding(cmd.ErrOrStderr(), dir, args[0])

			return nil
		},
	}

	cmd.Flags().BoolP(cli.YesFlagName, "y", false, "Skip the confirmation prompt.")
	cmd.Flags().String("dir", "", "Project directory whose manifest holds the binding, searched upward from here.")

	// Bind only the env var (DATAROBOT_CLI_NON_INTERACTIVE) to viper. The --yes
	// flag itself is read directly from cmd.Flags() so an explicit --yes does
	// not leak into viper.AllSettings() and persist to drconfig.yaml.
	_ = viperx.BindEnv(cli.YesFlagName, "DATAROBOT_CLI_NON_INTERACTIVE")

	telemetry.TrackWith(cmd, func(cmd *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"workload_id": telemetry.FirstArg(args),
			"yes":         cli.IsNonInteractive(cmd),
		}
	})

	return cmd
}

// confirmDelete returns (true, nil) when the deletion may proceed: either
// --yes / DATAROBOT_CLI_NON_INTERACTIVE was given, or the user confirmed
// interactively. A declined prompt is (false, nil) so the command exits 0
// as a no-op.
func confirmDelete(cmd *cobra.Command, workloadID string) (bool, error) {
	if cli.IsNonInteractive(cmd) {
		return true, nil
	}

	if !reader.IsStdinTerminal() {
		return false, errors.New("confirmation required: pass --yes (or set DATAROBOT_CLI_NON_INTERACTIVE=1) to delete without a prompt")
	}

	confirmed, err := helpers.Confirm(cmd.OutOrStdout(), cmd.InOrStdin(),
		"Delete workload "+workloadID+"? This stops and removes a running workload. [y/N] ")
	if err != nil {
		return false, err
	}

	if !confirmed {
		fmt.Println(tui.DimStyle.Render("Aborted."))
	}

	return confirmed, nil
}

// handleDeleteError converts a 404 into a friendly informational message
// (returns nil) so the user does not see a stack-trace-style HTTP error
// for what is effectively a no-op.
//
// The binding is deliberately left alone here. A 404 says the workload is not
// on this instance, which is not the same as saying it is gone: the same
// manifest read against the wrong endpoint, organisation or token answers
// exactly this way. `up` treats that ambiguity by naming the id and letting
// the user look; rewriting a committed file on the strength of it, in the one
// branch where nothing was actually deleted, would be the stronger claim made
// on the weaker evidence. A binding that really is dead is cleared by the
// deploy that recreates it.
func handleDeleteError(err error, workloadID string) error {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		fmt.Println(tui.DimStyle.Render("No workload found with id: " + workloadID))

		return nil
	}

	return err
}

// clearStaleBinding takes back the workloadId the CLI wrote into a manifest,
// now that the workload it names has been deleted.
//
// The id is matched inside the edit itself, so this only ever touches a file
// that is demonstrably about the workload just deleted, and the value it
// checks is the value it removes. A manifest naming some other workload is
// none of its business.
//
// dir is where to start looking, and it exists because Locate only walks
// upward. A project deployed with `up --dir site` and then deleted from the
// repository root is invisible from there, which is the reported bug's own
// reproduction; --dir is how the two commands agree on which project this is.
// A path that is not a directory is a typo worth naming, because Locate would
// otherwise walk past it to an ancestor and quietly edit the wrong project.
//
// Nothing here can fail the command. The workload is already gone by the time
// this runs, so a manifest that cannot be found, read or written is stepped
// over rather than turned into a failure for an operation that succeeded. The
// unwritable case still says so, because the user has to finish it by hand.
func clearStaleBinding(w io.Writer, dir, workloadID string) {
	if dir == "" {
		dir = "."
	}

	path, err := manifest.Locate(dir)
	if err != nil {
		// Locate is the one place that decides what counts as a directory, so
		// its answer is used rather than re-derived here. Reaching this needs
		// the directory to have gone away between the check that runs before
		// the delete and this line, which is rare and still worth a word: the
		// binding was not looked at, so it may well still be there.
		if errors.Is(err, manifest.ErrNotADirectory) {
			fmt.Fprintln(w, tui.DimStyle.Render("No manifest was checked: "+err.Error()+"."))
		}

		return
	}

	cleared, err := manifest.ClearWorkloadID(path, workloadID)

	// A file that cannot be parsed cannot be checked, so there is nothing to
	// say: it may not be this project's manifest at all, and `up` reports an
	// unreadable one in its own words. A failure past that point is on a file
	// whose binding was matched, so it is this command's business to report.
	if errors.Is(err, manifest.ErrUnreadable) {
		return
	}

	// The manifest package's refusals each carry the remedy that fits them,
	// and they differ: one says delete the file, another says edit it by hand.
	// Appending a single generic "remove that line" walked the user into the
	// empty manifest the only-key refusal exists to prevent.
	if err != nil {
		fmt.Fprintln(w, tui.DimStyle.Render("Could not remove workloadId from "+path+": "+err.Error()))

		return
	}

	if !cleared {
		return
	}

	fmt.Fprintln(w, tui.DimStyle.Render(
		"Removed workloadId from "+path+"; the next 'dr workload up"+manifest.DirFlag(dir)+"' creates a new workload."))

	noteLinkedArtifact(w, filepath.Dir(path))
}

// noteLinkedArtifact names the artifact this project is linked to, and how to
// stop being linked to it.
//
// Deleting a workload leaves its artifact alone, by design and as this
// command's help promises, so the link under .datarobot/ is still accurate and
// is deliberately left in place. It is only surprising when it is invisible,
// which is what this line fixes.
//
// It states that the artifact survived and stops there. Saying the next deploy
// reuses it would be false twice over: a locked artifact makes the next deploy
// refuse instead, and a manifest naming a published image or an artifactId
// never consults the link at all.
//
// The remedy is the state directory, not `dr artifact delete`. Deleting the
// artifact is refused outright while it is locked, and for an unlocked one it
// leaves the link pointing at something gone, which the next deploy reports as
// a bare 404 naming no fix. Removing the directory is what `up` itself already
// tells the user to do when a locked artifact blocks a deploy, so this says
// the same thing rather than inventing a second answer.
func noteLinkedArtifact(w io.Writer, projectDir string) {
	if !wapi.Exists(projectDir) {
		return
	}

	cfg, err := wapi.LoadConfig(projectDir)
	if err != nil || cfg.ArtifactID == "" {
		return
	}

	fmt.Fprintln(w, tui.DimStyle.Render(
		"This project is still linked to artifact "+cfg.ArtifactID+", which was not deleted with the workload. "+
			"To unlink it, delete "+wapi.Dir(projectDir)+"."))
}
