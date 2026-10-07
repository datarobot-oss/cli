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
	"path/filepath"
	"strings"

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

// The platform calls this command makes, as seams the tests replace: a real
// teardown must never reach a tenant from a unit test. deleteWorkloadFn is a
// seam too so an Execute()-level test can drive the whole RunE wiring — the
// read before the delete, the cleanup gate, the binding clear — without a
// server behind it.
var (
	getWorkloadFn           = workload.GetWorkload
	deleteWorkloadFn        = workload.DeleteWorkload
	credentialsWithPrefixFn = workload.CredentialsWithPrefix
	deleteCredentialFn      = workload.DeleteCredential
)

// credentialCleanupNoLimit walks the whole credential store rather than a
// bounded slice of it. The placeholder-message lookup passes a positive bound
// because it only wants one id and a large tenant should not turn that into a
// long walk; teardown is the opposite, because a scan that stopped at a page
// boundary would leave behind the very orphaned credentials it runs to remove.
const credentialCleanupNoLimit = 0

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

The credentials the CLI created for the workload (named
'<workload-name>/<env-var>') are deleted with it when the .datarobot.yaml for
that workload is found from --dir: only the credentials that manifest references
are removed, so the name is free to be reused. Run by id with no such manifest,
the matching credentials are listed for you to remove by hand rather than deleted
on a name match, which could belong to another workload that reused it. A
credential the platform will not remove because something else still uses it is
named rather than forced.

If the .datarobot.yaml found from --dir (the current directory by default,
searched upward from there) is bound to the workload being deleted, its
workloadId is removed too, so the project stops pointing at a workload that
is gone. Only a manifest naming that exact id is touched. Pass the same
--dir you deployed with: a manifest in a subdirectory is not visible from
its parent.

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

			return runDelete(cmd, ref)
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

// runDelete carries out a confirmed deletion: read the workload for its name and
// the manifest for the credentials to scope against, delete the workload, clean
// up its credentials, and repair the manifest that pointed at both.
func runDelete(cmd *cobra.Command, ref idargs.Ref) error {
	// Read the workload before deleting it: its name is the prefix of the
	// credentials to clean up afterwards, and a workload that is gone cannot be
	// read back for it. A lookup that fails is not fatal — the delete still runs.
	wl, getErr := getWorkloadFn(ref.ID)

	// Read the manifest's credential ids before the delete too, so cleanup
	// removes only the credentials this project points at and not every
	// credential that merely shares the workload's name prefix — one of those can
	// belong to a different workload that reused it. A delete by typed id from
	// outside any project finds no manifest and falls back to the prefix alone.
	scopeIDs, haveManifest := projectCredentialIDs(ref.Dir, ref.ID)

	if err := deleteWorkloadFn(ref.ID); err != nil {
		return handleDeleteError(err, ref)
	}

	fmt.Println(tui.BaseTextStyle.Render("Deleted workload: " + ref.ID))

	deleted := cleanupAfterDelete(cmd.ErrOrStderr(), ref, wl, getErr, scopeIDs, haveManifest)

	clearStaleBinding(cmd.ErrOrStderr(), ref.Dir, ref.ID, deleted)

	return nil
}

// cleanupAfterDelete removes the deleted workload's credentials and returns the
// ids it removed, so the manifest entries that pointed at them can be reset.
//
// A read that failed before the delete leaves the workload's name — the
// credential prefix — unknown, so the cleanup the prompt promised cannot run.
// The delete still succeeded, so this says what is left to finish by hand rather
// than failing, matching the list-failure branch in cleanupCredentials. A 404 is
// the exception: there was no workload, so none of its credentials either.
func cleanupAfterDelete(
	w io.Writer, ref idargs.Ref, wl *workload.Workload, getErr error, scopeIDs map[string]bool, haveManifest bool,
) []string {
	switch {
	case getErr == nil && wl.Name != "":
		return cleanupCredentials(w, wl.Name, scopeIDs, haveManifest)
	case getErr != nil && !drapi.IsNotFound(getErr):
		fmt.Fprintln(w, tui.DimStyle.Render(
			"Could not read workload "+ref.ID+" before deleting it, so its credentials were not cleaned up: "+
				getErr.Error()+". Remove any <workload-name>/* credentials by hand before reusing the name."))
	}

	return nil
}

// deleteConsequence is what agreeing to this question costs, and the one part
// of it `stop` and `start` have no equivalent of.
const deleteConsequence = "This stops and removes a running workload and deletes the credentials created for it."

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
	if drapi.IsNotFound(err) {
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
func clearStaleBinding(w io.Writer, dir, workloadID string, deletedCredIDs []string) {
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

	// Reset the references to the credentials just deleted before clearing the
	// binding. Leaving them pointing at ids that no longer exist is the other
	// half of the reported bug: the next deploy reads them in verifyCredentials
	// and fails on an id the user never typed, where a placeholder fails as an
	// entry the re-import knows how to finish. This is a separate edit to the
	// same file, so it runs whether or not the binding itself needs clearing.
	resetDeletedCredentials(w, path, deletedCredIDs)

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
			"; this project no longer points at a workload."))

	noteLinkedArtifact(w, filepath.Dir(path))
}

// projectCredentialIDs returns the set of credential ids the manifest under dir
// references, and whether that manifest is one to scope the cleanup by. The
// delete uses the set to scope its credential cleanup to this project's own
// credentials, and the bool to decide what to do otherwise: fall back rather than
// scope to an empty set and delete nothing.
//
// wantID is the id of the workload being deleted, and the manifest is only
// trusted when its workloadId is that same id — the guard clearStaleBinding
// already applies before it edits the file. Deleting workload B by typed id from
// inside project A would otherwise scope B's cleanup to A's credential ids and
// skip B's own; a manifest about a different workload is treated as no manifest.
//
// It is read-only and best-effort. A manifest that cannot be located, read or
// compiled, or that names a different workload, answers (nil, false), which is
// the same "no scope to apply" the fallback already handles — the realistic case
// this scopes, deleting a workload from the project it was deployed from, reads
// cleanly. Placeholder ids are dropped: they name no credential to delete.
func projectCredentialIDs(dir, wantID string) (map[string]bool, bool) {
	if dir == "" {
		dir = "."
	}

	path, err := manifest.Locate(dir)
	if err != nil {
		return nil, false
	}

	m, err := manifest.Load(path)
	if err != nil {
		return nil, false
	}

	// A manifest about some other workload says nothing about which credentials
	// this one owns, so it is no basis to scope by.
	if m.WorkloadID() != wantID {
		return nil, false
	}

	compiled, err := m.Compile()
	if err != nil {
		return nil, false
	}

	ids := make(map[string]bool, len(compiled.CredentialRefs))

	for _, ref := range compiled.CredentialRefs {
		if ref.CredentialID != "" && ref.CredentialID != manifest.CredentialPlaceholder {
			ids[ref.CredentialID] = true
		}
	}

	return ids, true
}

// resetDeletedCredentials turns the manifest references to the credentials just
// deleted back into placeholders. Like the rest of clearStaleBinding it cannot
// fail the command: the workload is already gone, so a file that cannot be read
// is stepped over, and one that cannot be written is reported with the remedy
// the user has to finish by hand.
func resetDeletedCredentials(w io.Writer, path string, deletedCredIDs []string) {
	if len(deletedCredIDs) == 0 {
		return
	}

	reset, err := manifest.ResetCredentialIDs(path, deletedCredIDs)

	// An unreadable file is stepped over in silence, the same as the binding
	// clear does: it may not be this project's manifest, and `up` reports such a
	// file in its own words.
	if errors.Is(err, manifest.ErrUnreadable) {
		return
	}

	if err != nil {
		fmt.Fprintln(w, tui.DimStyle.Render(
			"Could not reset the deleted credential references in "+idargs.DisplayPath(path)+": "+err.Error()+
				". Set them back to "+manifest.CredentialPlaceholder+" by hand before the next deploy."))

		return
	}

	if reset == 0 {
		return
	}

	fmt.Fprintln(w, tui.DimStyle.Render(
		"Reset "+credentialNoun(reset)+" in "+idargs.DisplayPath(path)+" to "+
			manifest.CredentialPlaceholder+"; store the values again before the next deploy."))
}

// credentialNoun agrees the count with its noun, so the reset message does not
// say "1 credential references".
func credentialNoun(n int) string {
	if n == 1 {
		return "1 credential reference"
	}

	return fmt.Sprintf("%d credential references", n)
}

// cleanupCredentials removes the credentials the CLI minted for the workload,
// which all carry the "<workloadName>/" prefix (see wizard.CredentialName), and
// returns the ids it removed so the manifest entries that pointed at them can be
// reset. Deleting them is what lets the name be reused: a credential the platform
// still holds under "<workloadName>/OPENAI_API_KEY" makes the next deploy of a
// workload by that name collide on it, which is the bug this fixes.
//
// scopeIDs and scoped decide what happens. When scoped is true a credential is
// deleted only if this project's manifest references it, so a credential that
// merely shares the name prefix but belongs to a different workload (one that
// reused it on a name collision) is left in place and named. When scoped is
// false — a delete by typed id with no manifest about this workload to consult —
// nothing is deleted: the prefix is not ownership, and the platform does not
// refuse to remove a credential a workload still references, so the matches are
// listed for the user to remove by hand rather than deleted on a guess.
//
// Like clearStaleBinding, nothing here can fail the command. The workload is
// already gone, so a lookup or delete that fails is reported — with the error
// text and what the user has to finish by hand — and stepped over rather than
// turned into a failure for a delete that succeeded. A credential the platform
// already lost (a 404) counts as removed, so its stale manifest reference is
// reset with the rest. One the platform refuses to remove (a 409, still used by
// a data connection or batch prediction job, or any other error) is named: the
// remedy is the user's, not ours to force.
func cleanupCredentials(w io.Writer, workloadName string, scopeIDs map[string]bool, scoped bool) []string {
	prefix := workloadName + "/"

	creds, err := credentialsWithPrefixFn(prefix, credentialCleanupNoLimit)
	if err != nil {
		fmt.Fprintln(w, tui.DimStyle.Render(
			"Could not list credentials to clean up for workload "+workloadName+": "+err.Error()+
				". Remove any "+prefix+"* credentials by hand before reusing this name."))

		return nil
	}

	if len(creds) == 0 {
		return nil
	}

	// Without a manifest about this workload, the name prefix is the only signal,
	// and it is not ownership: a credential sharing it can belong to a different
	// workload that reused it, and the platform does not refuse to delete one a
	// workload still references. Deleting on a prefix match alone would be a
	// destructive guess, so the matches are listed for the user to remove by hand.
	if !scoped {
		reportUnscopedMatches(w, prefix, creds)

		return nil
	}

	return deleteScopedCredentials(w, scopeIDs, creds)
}

// deleteScopedCredentials deletes the credentials scopeIDs names, returns their
// ids so the manifest references can be reset, and reports the rest: a delete
// that failed (with the error text), and a prefix match this project does not
// reference (named, not deleted). A 404 counts as deleted — the platform already
// lost it, but the manifest may still point at it.
func deleteScopedCredentials(w io.Writer, scopeIDs map[string]bool, creds []workload.Credential) []string {
	var (
		deleted []string
		failed  []string
		skipped []string
	)

	for _, c := range creds {
		switch {
		case !scopeIDs[c.CredentialID]:
			skipped = append(skipped, c.Name)
		case deleteOneCredential(w, c, &failed):
			deleted = append(deleted, c.CredentialID)
		}
	}

	for _, msg := range failed {
		fmt.Fprintln(w, tui.DimStyle.Render(
			"Could not delete credential "+msg+". It may still be in use; "+
				"remove it by hand before reusing this workload name."))
	}

	if len(skipped) > 0 {
		fmt.Fprintln(w, tui.DimStyle.Render(fmt.Sprintf(
			"Left %d credential(s) in place that this project does not reference (%s); "+
				"another workload may use them — remove by hand if not.",
			len(skipped), strings.Join(skipped, ", "))))
	}

	return deleted
}

// deleteOneCredential removes one credential and reports whether its id should be
// counted as removed (deleted, or a 404 the platform already lost). A refusal is
// appended to failed with its error text rather than printed here, so all the
// failures read together at the end.
func deleteOneCredential(w io.Writer, c workload.Credential, failed *[]string) bool {
	err := deleteCredentialFn(c.CredentialID)

	switch {
	case err == nil:
		fmt.Fprintln(w, tui.DimStyle.Render("Deleted credential "+c.Name+"."))

		return true
	case drapi.IsNotFound(err):
		// Already gone on the platform's side. There is nothing left to remove,
		// but the manifest may still point at it, so it counts as deleted for the
		// reset that follows.
		return true
	default:
		*failed = append(*failed, c.Name+": "+err.Error())

		return false
	}
}

// reportUnscopedMatches names the prefix matches found when there is no manifest
// to confirm the workload owns them, and tells the user to remove them by hand.
// It deletes nothing: see cleanupCredentials on why a prefix match alone is not
// safe to delete.
func reportUnscopedMatches(w io.Writer, prefix string, creds []workload.Credential) {
	names := make([]string, len(creds))
	for i, c := range creds {
		names[i] = c.Name
	}

	fmt.Fprintln(w, tui.DimStyle.Render(fmt.Sprintf(
		"Found %d credential(s) named %s* but no manifest for this workload to confirm it owns them (%s). "+
			"They are left in place because the name can be shared with another workload; "+
			"remove them by hand once you have checked nothing else uses them.",
		len(creds), prefix, strings.Join(names, ", "))))
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
