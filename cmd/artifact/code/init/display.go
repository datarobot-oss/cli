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

func printAlreadyLinked(artifactID, dir string) {
	stateDir := wapi.Dir(dir)

	fmt.Println(tui.ErrorStyle.Render(
		fmt.Sprintf("Already linked to artifact %s; state exists at %s.", artifactID, stateDir),
	))
	fmt.Println(tui.DimStyle.Render(fmt.Sprintf("Delete %s to re-init.", stateDir)))
}

func shortVer(s string) string {
	if len(s) > 8 {
		return s[:8]
	}

	return s
}
