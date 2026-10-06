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

package del

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/datarobot/cli/cmd/workload/internal/idargs"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/datarobot/cli/internal/workload/wizard"
	"github.com/datarobot/cli/tui"
)

// Seams so tests stay off the network.
var (
	getWorkloadFn      = workload.GetWorkload
	getArtifactFn      = workload.GetArtifact
	deleteArtifactFn   = workload.DeleteArtifact
	getCredentialFn    = workload.GetCredential
	deleteCredentialFn = workload.DeleteCredential
)

const purgeConsequence = "--purge also removes the artifact it ran, " +
	"the credentials this project minted, and the local state directory."

// purgeSet is what a purge is going to remove, worked out before the workload
// is deleted: the artifact it ran, and the project whose manifest named it,
// which is where the credentials and the state directory are found. Reason
// says why nothing will be, "" when something will.
type purgeSet struct {
	workloadID   string
	artifactID   string
	manifestPath string
	projectDir   string
	reason       string
}

// purgeReport is what a purge removed and what it kept, each with the reason.
type purgeReport struct {
	removed []string
	kept    []string
}

// planPurge reads what the purge needs while the workload still exists: the
// artifact it runs, and the manifest that names it. A workload that cannot be
// read stops the delete, since the purge was the point of the run.
func planPurge(ref idargs.Ref) (purgeSet, error) {
	w, err := getWorkloadFn(ref.ID)
	if err != nil {
		return purgeSet{}, fmt.Errorf("cannot read workload %s to find its leftovers, so nothing was deleted: %w", ref.ID, err)
	}

	set := purgeSet{workloadID: ref.ID, artifactID: w.ArtifactID}

	path := ref.Path
	if path == "" {
		dir := ref.Dir
		if dir == "" {
			dir = "."
		}

		if path, err = manifest.Locate(dir); err != nil {
			set.reason = "no manifest here names workload " + ref.ID + ", so there is nothing to tie its leftovers to"

			return set, nil
		}
	}

	m, err := manifest.Load(path)
	if err != nil || m.WorkloadID() != ref.ID {
		set.reason = "no manifest here names workload " + ref.ID + ", so there is nothing to tie its leftovers to"

		return set, nil
	}

	set.manifestPath, set.projectDir = path, filepath.Dir(path)

	return set, nil
}

// runPurge removes what the deploy created beside the workload. Nothing here
// fails the command, since the workload is already gone: what cannot be
// removed is named.
func runPurge(w io.Writer, set purgeSet) {
	if set.reason != "" {
		fmt.Fprintln(w, tui.WarnStyle.Render("Nothing purged: "+set.reason+"."))

		return
	}

	var report purgeReport

	// The artifact goes first: one that survives, for whatever reason, may
	// still be run with these credentials baked into its spec.
	if purgeArtifact(set.artifactID, &report) {
		report.kept = append(report.kept, "the credentials: artifact "+set.artifactID+" survived, and whatever runs it may read them")
	} else {
		purgeCredentials(set.manifestPath, &report)
	}

	purgeState(set.projectDir, &report)

	for _, line := range report.removed {
		fmt.Fprintln(w, tui.DimStyle.Render("Removed "+line))
	}

	for _, line := range report.kept {
		fmt.Fprintln(w, tui.WarnStyle.Render("Kept "+line))
	}
}

// purgeCredentials deletes the referenced credentials named <workload>/<ENV>,
// the ones this project minted, and puts the placeholder back into the
// manifest for each one gone, so the entry reads as unfinished again and the
// next --sync-env finishes it. Any other credential may be shared, so it is
// kept.
func purgeCredentials(manifestPath string, report *purgeReport) {
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return
	}

	compiled, err := m.Compile()
	if err != nil {
		return
	}

	// Without a name, the minted pattern collapses to the bare variable name,
	// which is exactly the shared credential this must not delete.
	if m.Name() == "" && len(compiled.CredentialRefs) > 0 {
		report.kept = append(report.kept, "the credentials: the manifest has no name, so the ones this project minted "+
			"cannot be told from shared ones")

		return
	}

	gone := map[string]bool{}

	for _, ref := range compiled.CredentialRefs {
		if _, seen := gone[ref.CredentialID]; seen {
			continue
		}

		gone[ref.CredentialID] = purgeCredential(ref, wizard.CredentialName(m.Name(), ref.EnvName), report)
	}

	resetReferences(manifestPath, gone, report)
}

// purgeCredential deletes one referenced credential when it carries the
// minted name, and reports whether the reference now points at nothing.
func purgeCredential(ref manifest.CredentialRef, minted string, report *purgeReport) (gone bool) {
	cred, err := getCredentialFn(ref.CredentialID)
	if err != nil {
		if statusIs(err, http.StatusNotFound) {
			report.removed = append(report.removed, fmt.Sprintf("the reference to credential %s for %s, which was already gone",
				ref.CredentialID, ref.EnvName))

			return true
		}

		report.kept = append(report.kept, fmt.Sprintf("credential %s for %s: could not read it: %v",
			ref.CredentialID, ref.EnvName, err))

		return false
	}

	if cred.Name != minted {
		report.kept = append(report.kept, fmt.Sprintf(
			"credential %s (%s) for %s: not minted by this project, so it may be shared; "+
				"delete it in the DataRobot UI if it is yours", cred.CredentialID, cred.Name, ref.EnvName))

		return false
	}

	if err := deleteCredentialFn(cred.CredentialID); err != nil {
		report.kept = append(report.kept, fmt.Sprintf("credential %s (%s): %v", cred.CredentialID, cred.Name, err))

		return false
	}

	report.removed = append(report.removed, fmt.Sprintf("credential %s (%s)", cred.CredentialID, cred.Name))

	return true
}

// resetReferences puts the placeholder back for every credential that is
// gone, so the manifest does not name credentials that no longer exist.
func resetReferences(manifestPath string, gone map[string]bool, report *purgeReport) {
	ids := map[string]bool{}

	for id, isGone := range gone {
		if isGone {
			ids[id] = true
		}
	}

	if len(ids) == 0 {
		return
	}

	n, err := manifest.ResetCredentialReferences(manifestPath, ids)
	if err != nil {
		report.kept = append(report.kept, fmt.Sprintf("the references in %s to the credentials just removed: %v; "+
			"replace each id with %s by hand", idargs.DisplayPath(manifestPath), err, manifest.CredentialPlaceholder))

		return
	}

	report.removed = append(report.removed, fmt.Sprintf("%d credential %s in %s, reset to %s for the next --sync-env to fill",
		n, plural(n, "reference", "references"), idargs.DisplayPath(manifestPath), manifest.CredentialPlaceholder))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}

	return many
}

// purgeArtifact deletes the artifact the workload ran, unless it is locked or
// another workload still references it, and reports whether it survived.
func purgeArtifact(id string, report *purgeReport) (survives bool) {
	if id == "" {
		return false
	}

	artifact, err := getArtifactFn(id)
	if err != nil {
		if statusIs(err, http.StatusNotFound) {
			report.removed = append(report.removed, "artifact "+id+", which was already gone")

			return false
		}

		report.kept = append(report.kept, fmt.Sprintf("artifact %s: could not read it: %v", id, err))

		return true
	}

	if artifact.IsLocked() {
		report.kept = append(report.kept, "artifact "+id+": it is locked, and a locked artifact cannot be deleted")

		return true
	}

	if err := deleteArtifactFn(id); err != nil {
		if statusIs(err, http.StatusConflict) {
			report.kept = append(report.kept, "artifact "+id+": another workload still references it; "+
				"run 'dr artifact delete "+id+"' once nothing does")

			return true
		}

		report.kept = append(report.kept, fmt.Sprintf("artifact %s: %v", id, err))

		return true
	}

	report.removed = append(report.removed, "artifact "+id)

	return false
}

// purgeState removes the local state directory, so the next deploy links a
// fresh artifact.
func purgeState(projectDir string, report *purgeReport) {
	// Both locations: a legacy tree left beside the current one would be
	// found again by the next deploy once the current one is gone.
	for _, dir := range wapi.StateDirs(projectDir) {
		if err := os.RemoveAll(dir); err != nil {
			report.kept = append(report.kept, fmt.Sprintf("%s: %v; remove it by hand", dir, err))

			continue
		}

		report.removed = append(report.removed, dir)
	}
}

func statusIs(err error, code int) bool {
	var httpErr *drapi.HTTPError

	return errors.As(err, &httpErr) && httpErr.StatusCode == code
}
