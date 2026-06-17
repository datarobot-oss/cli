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

func printAlreadyLinked(artifactID, dir string) {
	fmt.Println(tui.ErrorStyle.Render(
		fmt.Sprintf("Already linked to artifact %s; .wapi/ exists at %s.", artifactID, dir),
	))
	fmt.Println(tui.DimStyle.Render("Delete .wapi/ to re-init."))
}

func shortVer(s string) string {
	if len(s) > 8 {
		return s[:8]
	}

	return s
}
