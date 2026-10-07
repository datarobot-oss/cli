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

package initcmd

import (
	"encoding/json"
	"fmt"
	"io"

	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/datarobot/cli/tui"
)

type initResult struct {
	ArtifactID       string  `json:"artifactId"`
	Name             string  `json:"name"`
	Status           string  `json:"status"`
	CatalogID        *string `json:"catalogId"`
	CatalogVersionID *string `json:"catalogVersionId"`
	Dir              string  `json:"dir"`
}

// alreadyLinkedJSON is the pinned JSON shape emitted on stdout when init
// aborts because the project is already linked. Human-readable text goes to
// stderr; stdout stays pure JSON.
type alreadyLinkedJSON struct {
	Status     string  `json:"status"`
	Error      string  `json:"error"`
	ArtifactID *string `json:"artifactId"`
	Remedy     string  `json:"remedy"`
}

// relinkJSONResult describes a successful relink from the init offer in JSON
// mode. Stdout is pure JSON; all prompt/warning text went to stderr.
type relinkJSONResult struct {
	Status     string        `json:"status"`
	ArtifactID string        `json:"artifactId"`
	Actions    []core.Action `json:"actions"`
}

func newInitResult(art workload.Artifact, dir string) initResult {
	r := initResult{
		ArtifactID: art.ID,
		Name:       art.Name,
		Status:     art.Status,
		Dir:        dir,
	}

	if codeRef := workload.ExtractCodeRef(art); codeRef != nil {
		r.CatalogID = &codeRef.CatalogID
		r.CatalogVersionID = &codeRef.CatalogVersionID
	}

	return r
}

// renderInitResult reports the link on stdout, and in text mode what to run
// next on stderr, so a script capturing stdout gets the report alone.
func renderInitResult(stderr io.Writer, format outputformat.OutputFormat, result initResult) error {
	if format == outputformat.OutputFormatJSON {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(data))

		return nil
	}

	// The --dir goes along: init may have been pointed at, or prompted for, a
	// directory the shell is not standing in, and a bare sync run from here
	// would find no link.
	step := tui.NextStep{Command: "dr artifact code sync" + manifest.DirFlag(result.Dir)}

	if result.CatalogVersionID != nil {
		printLinkedExistingCode(result.Name, result.ArtifactID, shortVer(*result.CatalogVersionID))

		step.Description = "Reconcile any local changes"
	} else {
		printLinkedEmptyArtifact(result.Name, result.ArtifactID)

		step.Description = "Upload your files"
	}

	tui.PrintNextSteps(stderr, step)

	return nil
}

// renderAlreadyLinkedJSON emits the pinned abort shape on stdout. The caller
// is responsible for printing human-readable text to stderr and returning an
// error that drives the exit code. HTML escaping is disabled so remedy
// strings like "doctor --relink <new-artifact-id>" survive verbatim (matching
// the doctor's JSON reporter).
func renderAlreadyLinkedJSON(w io.Writer, artifactID *string, remedy string) {
	enc := json.NewEncoder(w)

	enc.SetEscapeHTML(false)

	_ = enc.Encode(alreadyLinkedJSON{
		Status:     "error",
		Error:      "already-linked",
		ArtifactID: artifactID,
		Remedy:     remedy,
	})
}

// renderRelinkJSON emits the relink result as pure JSON on stdout. HTML
// escaping is disabled for consistency with the doctor's JSON reporter.
func renderRelinkJSON(w io.Writer, artifactID string, actions []core.Action) {
	enc := json.NewEncoder(w)

	enc.SetEscapeHTML(false)

	_ = enc.Encode(relinkJSONResult{
		Status:     "ok",
		ArtifactID: artifactID,
		Actions:    actions,
	})
}

func printLinkedExistingCode(name, artifactID, verShort string) {
	fmt.Println(tui.SuccessStyle.Render(
		fmt.Sprintf("Linked to %s (%s) at version %s.", name, artifactID, verShort),
	))
}

func printLinkedEmptyArtifact(name, artifactID string) {
	fmt.Println(tui.SuccessStyle.Render(
		fmt.Sprintf("Linked to empty artifact %s (%s).", name, artifactID),
	))
}

// printAlreadyLinkedHealthy prints the already-linked abort message for a
// healthy linked artifact, pointing to the doctor for diagnosis. No delete
// advice.
func printAlreadyLinkedHealthy(w io.Writer, artifactID, dir string) {
	stateDir := wapi.Dir(dir)

	fmt.Fprintln(w, tui.ErrorStyle.Render(
		fmt.Sprintf("Already linked to artifact %s; state exists at %s.", artifactID, stateDir),
	))
	fmt.Fprintln(w, tui.DimStyle.Render(
		"Run 'dr artifact code doctor"+manifest.DirFlag(dir)+"' to diagnose the sync state."))
}

// printCorruptConfig prints the unreadable-config message, pointing to
// doctor --relink, which replaces the file. No delete advice.
func printCorruptConfig(w io.Writer, dir, artifactID string) {
	configPath := wapi.ConfigPath(dir)

	fmt.Fprintln(w, tui.ErrorStyle.Render(
		fmt.Sprintf("Project is already linked but the config at %s is unreadable.", configPath),
	))
	fmt.Fprintln(w, tui.DimStyle.Render(corruptConfigRemedy(dir, artifactID)))
}

// corruptConfigRemedy names the relink that replaces an unreadable config,
// with the artifact id init was given when there is one.
func corruptConfigRemedy(dir, artifactID string) string {
	return "Run '" + relinkRemedy(dir, artifactID) + "' to replace the config."
}

// relinkRemedy is the relink command for the JSON remedy field, with the
// artifact id init was given or a placeholder, and the --dir init ran with.
func relinkRemedy(dir, artifactID string) string {
	if artifactID == "" {
		artifactID = "<new-artifact-id>"
	}

	return "dr artifact code doctor --relink " + artifactID + manifest.DirFlag(dir)
}

// printGoneGuidance prints the non-interactive guidance for a gone artifact,
// pointing to doctor --relink. No delete advice.
func printGoneGuidance(w io.Writer, dir, artifactID, givenID string) {
	fmt.Fprintln(w, tui.ErrorStyle.Render(
		fmt.Sprintf("Already linked to artifact %s, but the artifact was not found (deleted?).", artifactID),
	))
	fmt.Fprintln(w, tui.DimStyle.Render(
		"Run '"+relinkRemedy(dir, givenID)+"' to relink to a new artifact.",
	))
}

// printMismatchGuidance prints the non-interactive guidance for a catalog
// mismatch, pointing to doctor --relink. No delete advice.
func printMismatchGuidance(w io.Writer, dir, artifactID, givenID string) {
	fmt.Fprintln(w, tui.ErrorStyle.Render(
		fmt.Sprintf("Already linked to artifact %s, but the catalog id no longer matches.", artifactID),
	))
	fmt.Fprintln(w, tui.DimStyle.Render(
		"Run '"+relinkRemedy(dir, givenID)+"' to relink to a new artifact.",
	))
}

// printRelinkSuccess prints the text-mode success message after a relink
// from the init offer completes.
func printRelinkSuccess(w io.Writer, dir, artifactID string) {
	fmt.Fprintln(w, tui.SuccessStyle.Render(
		fmt.Sprintf("Relinked to artifact %s; sync baseline reset.", artifactID),
	))
	fmt.Fprintln(w, tui.DimStyle.Render(
		"Run 'dr artifact code sync"+manifest.DirFlag(dir)+"' to reconcile against the new artifact."))
}

func shortVer(s string) string {
	if len(s) > 8 {
		return s[:8]
	}

	return s
}
