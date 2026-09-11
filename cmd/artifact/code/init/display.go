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

func renderInitResult(format outputformat.OutputFormat, result initResult) error {
	if format == outputformat.OutputFormatJSON {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(data))

		return nil
	}

	if result.CatalogVersionID != nil {
		printLinkedExistingCode(result.Name, result.ArtifactID, shortVer(*result.CatalogVersionID))

		return nil
	}

	printLinkedEmptyArtifact(result.Name, result.ArtifactID)

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
	fmt.Println(tui.DimStyle.Render("Run 'dr artifact code sync' to reconcile any local changes."))
}

func printLinkedEmptyArtifact(name, artifactID string) {
	fmt.Println(tui.SuccessStyle.Render(
		fmt.Sprintf("Linked to empty artifact %s (%s).", name, artifactID),
	))
	fmt.Println(tui.DimStyle.Render("Run 'dr artifact code sync' to upload your files."))
}

// printAlreadyLinkedHealthy prints the already-linked abort message for a
// healthy linked artifact, pointing to the doctor for diagnosis. No delete
// advice.
func printAlreadyLinkedHealthy(w io.Writer, artifactID, dir string) {
	stateDir := wapi.Dir(dir)

	fmt.Fprintln(w, tui.ErrorStyle.Render(
		fmt.Sprintf("Already linked to artifact %s; state exists at %s.", artifactID, stateDir),
	))
	fmt.Fprintln(w, tui.DimStyle.Render("Run 'dr artifact code doctor' to diagnose the sync state."))
}

// printCorruptConfig prints the unreadable-config message, pointing to
// doctor --fix. No delete advice.
func printCorruptConfig(w io.Writer, dir string) {
	configPath := wapi.ConfigPath(dir)

	fmt.Fprintln(w, tui.ErrorStyle.Render(
		fmt.Sprintf("Project is already linked but the config at %s is unreadable.", configPath),
	))
	fmt.Fprintln(w, tui.DimStyle.Render("Run 'dr artifact code doctor --fix' to repair the config."))
}

// printGoneGuidance prints the non-interactive guidance for a gone artifact,
// pointing to doctor --relink. No delete advice.
func printGoneGuidance(w io.Writer, artifactID string) {
	fmt.Fprintln(w, tui.ErrorStyle.Render(
		fmt.Sprintf("Already linked to artifact %s, but the artifact was not found (deleted?).", artifactID),
	))
	fmt.Fprintln(w, tui.DimStyle.Render(
		"Run 'dr artifact code doctor --relink <new-artifact-id>' to relink to a new artifact.",
	))
}

// printMismatchGuidance prints the non-interactive guidance for a catalog
// mismatch, pointing to doctor --relink. No delete advice.
func printMismatchGuidance(w io.Writer, artifactID string) {
	fmt.Fprintln(w, tui.ErrorStyle.Render(
		fmt.Sprintf("Already linked to artifact %s, but the catalog id no longer matches.", artifactID),
	))
	fmt.Fprintln(w, tui.DimStyle.Render(
		"Run 'dr artifact code doctor --relink <new-artifact-id>' to relink to a new artifact.",
	))
}

// printRelinkSuccess prints the text-mode success message after a relink
// from the init offer completes.
func printRelinkSuccess(w io.Writer, artifactID string) {
	fmt.Fprintln(w, tui.SuccessStyle.Render(
		fmt.Sprintf("Relinked to artifact %s; sync baseline reset.", artifactID),
	))
	fmt.Fprintln(w, tui.DimStyle.Render("Run 'dr artifact code sync' to reconcile against the new artifact."))
}

func shortVer(s string) string {
	if len(s) > 8 {
		return s[:8]
	}

	return s
}
