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
	"strings"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/datarobot/cli/internal/workload/wizard"
	"github.com/datarobot/cli/tui"
)

// Seams so tests stay off the network.
var (
	getArtifactFn      = workload.GetArtifact
	deleteArtifactFn   = workload.DeleteArtifact
	getCredentialFn    = workload.GetCredential
	deleteCredentialFn = workload.DeleteCredential
)

const purgeConsequence = "--purge also removes the artifact it ran, " +
	"the credentials this project minted, and the local state directory."

// purgeReport is what a purge removed and what it kept, each with the reason.
type purgeReport struct {
	removed []string
	kept    []string
}

// purgeLeftovers removes what the deploy created beside the workload. Nothing
// here fails the command, since the workload is already gone: what cannot be
// removed is named.
func purgeLeftovers(w io.Writer, projectDir, manifestPath string) {
	var report purgeReport

	purgeCredentials(manifestPath, &report)
	purgeArtifact(projectDir, &report)
	purgeState(projectDir, &report)

	for _, line := range report.removed {
		fmt.Fprintln(w, tui.DimStyle.Render("Removed "+line))
	}

	for _, line := range report.kept {
		fmt.Fprintln(w, tui.WarnStyle.Render("Kept "+line))
	}
}

// purgeCredentials deletes the referenced credentials named <workload>/<ENV>,
// the ones this project minted. Any other may be shared, so it is kept.
func purgeCredentials(manifestPath string, report *purgeReport) {
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return
	}

	compiled, err := m.Compile()
	if err != nil {
		return
	}

	seen := map[string]bool{}

	for _, ref := range compiled.CredentialRefs {
		if seen[ref.CredentialID] {
			continue
		}

		seen[ref.CredentialID] = true

		cred, err := getCredentialFn(ref.CredentialID)
		if err != nil {
			report.kept = append(report.kept, fmt.Sprintf("credential %s for %s: could not read it: %v",
				ref.CredentialID, ref.EnvName, err))

			continue
		}

		minted := wizard.CredentialName(m.Name(), ref.EnvName)
		if cred.Name != minted {
			report.kept = append(report.kept, fmt.Sprintf(
				"credential %s (%s) for %s: not minted by this project, so it may be shared; "+
					"delete it in the DataRobot UI if it is yours", cred.CredentialID, cred.Name, ref.EnvName))

			continue
		}

		if err := deleteCredentialFn(cred.CredentialID); err != nil {
			report.kept = append(report.kept, fmt.Sprintf("credential %s (%s): %v", cred.CredentialID, cred.Name, err))

			continue
		}

		report.removed = append(report.removed, fmt.Sprintf("credential %s (%s)", cred.CredentialID, cred.Name))
	}
}

// purgeArtifact deletes the linked artifact unless it is locked or another
// workload still references it.
func purgeArtifact(projectDir string, report *purgeReport) {
	cfg, err := wapi.LoadConfig(projectDir)
	if err != nil || cfg.ArtifactID == "" {
		return
	}

	id := cfg.ArtifactID

	artifact, err := getArtifactFn(id)
	if err != nil {
		if statusIs(err, http.StatusNotFound) {
			report.removed = append(report.removed, "the link to artifact "+id+", which was already gone")

			return
		}

		report.kept = append(report.kept, fmt.Sprintf("artifact %s: could not read it: %v", id, err))

		return
	}

	if artifact.IsLocked() {
		report.kept = append(report.kept, "artifact "+id+": it is locked, and a locked artifact cannot be deleted")

		return
	}

	if err := deleteArtifactFn(id); err != nil {
		if statusIs(err, http.StatusConflict) {
			report.kept = append(report.kept, "artifact "+id+": another workload still references it; "+
				"run 'dr artifact delete "+id+"' once nothing does")

			return
		}

		report.kept = append(report.kept, fmt.Sprintf("artifact %s: %v", id, err))

		return
	}

	report.removed = append(report.removed, "artifact "+id)
}

// purgeState removes the local state directory, so the next deploy links a
// fresh artifact.
func purgeState(projectDir string, report *purgeReport) {
	if !wapi.Exists(projectDir) {
		return
	}

	dir := wapi.Dir(projectDir)

	if err := os.RemoveAll(dir); err != nil {
		report.kept = append(report.kept, fmt.Sprintf("%s: %v; remove it by hand", dir, err))

		return
	}

	report.removed = append(report.removed, dir)
}

func statusIs(err error, code int) bool {
	var httpErr *drapi.HTTPError

	return errors.As(err, &httpErr) && httpErr.StatusCode == code
}

// purgeSummary says why a purge with no manifest naming the workload removed
// nothing more.
func purgeSummary(cleared bool, workloadID string) string {
	if cleared {
		return ""
	}

	return strings.TrimSpace("Nothing purged: no manifest here names workload " + workloadID +
		", so there is nothing to tie its leftovers to.")
}
