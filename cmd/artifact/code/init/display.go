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

	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/datarobot/cli/tui"
)

type initResult struct {
	ArtifactID string `json:"artifactId"`
	Name       string `json:"name"`
	Status     string `json:"status"`

	// PreviousArtifactID is the artifact a --force run re-pointed away from,
	// and nil for a fresh link, which had none.
	//
	// It is here because after the write nothing else remembers it: a caller
	// that re-pointed the wrong directory has the id it needs to put the link
	// back only if this field carried it out. The text path prints the same
	// thing, so leaving it off JSON made --output-format json the one way to
	// run the command that loses it.
	PreviousArtifactID *string `json:"previousArtifactId"`

	CatalogID        *string `json:"catalogId"`
	CatalogVersionID *string `json:"catalogVersionId"`
	Dir              string  `json:"dir"`
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

// printAlreadyLinked refuses, and names the command that changes the link.
//
// It used to end at "delete the state directory", which is filesystem surgery
// standing in for a command: it takes the project's ignore file and its history
// along with the one field being changed, and it is the same advice that made
// deleting state the folk cure for every stuck project.
func printAlreadyLinked(artifactID, dir string) {
	fmt.Println(tui.ErrorStyle.Render(
		fmt.Sprintf("Already linked to artifact %s; state exists at %s.", artifactID, wapi.Dir(dir)),
	))
	fmt.Println(tui.DimStyle.Render(
		"Pass --force to point this directory at another artifact instead."))
}

// printRelinked says what moved, and from where.
func printRelinked(previous, artifactID, dir string) {
	if previous == "" {
		fmt.Println(tui.SuccessStyle.Render(
			fmt.Sprintf("Pointed %s at artifact %s.", dir, artifactID)))

		return
	}

	fmt.Println(tui.SuccessStyle.Render(
		fmt.Sprintf("Re-pointed %s from artifact %s to %s.", dir, previous, artifactID)))
	fmt.Println(tui.DimStyle.Render(
		"The next sync uploads whatever the new artifact does not already hold."))
}

func shortVer(s string) string {
	if len(s) > 8 {
		return s[:8]
	}

	return s
}
