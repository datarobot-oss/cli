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

	"github.com/datarobot/cli/cmd/workload/internal/idargs"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var ref idargs.Ref

	cmd := &cobra.Command{
		Use:   "delete [<workload-id>]",
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

` + idargs.HelpText + `

Example:
  dr workload delete
  dr workload delete 68b0c1d2e3f4a5b6c7d8e9f0
  dr workload delete 68b0c1d2e3f4a5b6c7d8e9f0 --yes`,
		Args:         cobra.MaximumNArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error

			ref, err = idargs.Resolve(cmd, args)
			if err != nil {
				return err
			}

			confirmed, err := confirmDelete(cmd, ref)
			if err != nil || !confirmed {
				return err
			}

			if err := workload.DeleteWorkload(ref.ID); err != nil {
				return handleDeleteError(err, ref)
			}

			fmt.Println(tui.BaseTextStyle.Render("Deleted workload: " + ref.ID))

			clearStaleBinding(cmd.ErrOrStderr(), ref.Dir, ref.ID)

			return nil
		},
	}

	idargs.AddDirFlag(cmd)
	// Only here does --dir also decide which manifest gets its binding cleared,
	// which is the whole reason a delete by typed id still takes the flag.
	cmd.Flags().Lookup("dir").Usage += " Its binding to the deleted workload is cleared too."

	idargs.AddYesFlag(cmd, "Skip the confirmation prompt.")

	telemetry.TrackWith(cmd, func(cmd *cobra.Command, args []string) map[string]any {
		yesFlag, _ := cmd.Flags().GetBool(cli.YesFlagName)

		return map[string]any{
			"workload_id":        idargs.TelemetryID(ref, args),
			"workload_id_source": ref.Source,
			// The environment variable is only consent for a workload the user
			// named, so reporting it unconditionally would say yes about the
			// runs this command refuses for want of it.
			"yes": yesFlag || (!ref.FromManifest() && cli.IsNonInteractive(cmd)),
		}
	})

	return cmd
}

// confirmDelete returns (true, nil) when the deletion may proceed: either it
// was already answered, or the user confirmed interactively. A declined prompt
// is (false, nil) so the command exits 0 as a no-op.
//
// A workload the manifest specified rather than the user takes the explicit
// --yes only. Delete is the one irreversible verb here, and the environment
// variable that suppresses wizards in CI should not also stand as consent to
// remove something nobody typed.
func confirmDelete(cmd *cobra.Command, ref idargs.Ref) (bool, error) {
	env := idargs.EnvMayConsent
	if ref.FromManifest() {
		env = idargs.EnvMayNotConsent
	}

	return idargs.Confirm(cmd, deleteQuestion(ref), env)
}

// deleteConsequence is what agreeing to this question costs, and the one part
// of it `stop` and `start` have no equivalent of.
const deleteConsequence = "This stops and removes a running workload."

// deleteQuestion is the question itself, split out because Confirm refuses
// before printing anything when there is no terminal, which is every test.
func deleteQuestion(ref idargs.Ref) string {
	return idargs.Prompt("Delete", ref, deleteConsequence)
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
func handleDeleteError(err error, ref idargs.Ref) error {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		// The manifest is named when it, rather than the user, chose the id:
		// otherwise this reports an id the reader has never seen and gives
		// them nowhere to look.
		said := "No workload found with id: " + ref.ID
		if ref.FromManifest() {
			said += ref.SpecifiedIn()
		}

		fmt.Println(tui.DimStyle.Render(said))

		return nil
	}

	// Anything else keeps the provenance too, which is what turns a bare 403
	// against an ambient id into something actionable.
	return ref.Wrap(err)
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
		fmt.Fprintln(w, tui.DimStyle.Render(
			"Could not remove workloadId from "+idargs.DisplayPath(path)+": "+err.Error()))

		return
	}

	if !cleared {
		return
	}

	fmt.Fprintln(w, tui.DimStyle.Render(
		"Removed workloadId from "+idargs.DisplayPath(path)+
			"; the next 'dr workload up"+manifest.DirFlag(dir)+"' creates a new workload."))

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
