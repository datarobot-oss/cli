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

package workload

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
)

const (
	timestampFormat       = "2006-01-02 15:04 UTC"
	emptyValuePlaceholder = "—"
)

func RenderArtifact(format OutputFormat, artifact Artifact) error {
	if format == OutputFormatJSON {
		return printArtifactJSON(artifact)
	}

	printArtifactDetails(artifact)

	return nil
}

func RenderArtifacts(format OutputFormat, artifacts []Artifact) error {
	if format == OutputFormatJSON {
		return printArtifactsJSON(artifacts)
	}

	printArtifactsTable(artifacts)

	return nil
}

func printArtifactJSON(artifact Artifact) error {
	data, err := json.MarshalIndent(NewArtifactOutput(artifact), "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

func printArtifactsJSON(artifacts []Artifact) error {
	outputs := make([]ArtifactOutput, 0, len(artifacts))

	for _, a := range artifacts {
		outputs = append(outputs, NewArtifactOutput(a))
	}

	data, err := json.MarshalIndent(outputs, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

func printArtifactDetails(artifact Artifact) {
	catalogID, versionID := emptyValuePlaceholder, emptyValuePlaceholder

	if codeRef := ExtractCodeRef(artifact); codeRef != nil {
		catalogID = codeRef.CatalogID
		versionID = codeRef.CatalogVersionID
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "ID:\t%s\n", artifact.ID)
	fmt.Fprintf(w, "Name:\t%s\n", artifact.Name)
	fmt.Fprintf(w, "Status:\t%s\n", artifact.Status)
	fmt.Fprintf(w, "Catalog ID:\t%s\n", catalogID)
	fmt.Fprintf(w, "Version ID:\t%s\n", versionID)
	fmt.Fprintf(w, "Created:\t%s\n", artifact.CreatedAt.UTC().Format(timestampFormat))
	fmt.Fprintf(w, "Updated:\t%s\n", artifact.UpdatedAt.UTC().Format(timestampFormat))

	w.Flush()
}

func printArtifactsTable(artifacts []Artifact) {
	if len(artifacts) == 0 {
		fmt.Println("No artifacts found.")

		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintln(w, "ARTIFACT ID\tNAME\tSTATUS\tCATALOG ID\tVERSION ID\tUPDATED")

	for _, a := range artifacts {
		catalogID, versionID := emptyValuePlaceholder, emptyValuePlaceholder

		if codeRef := ExtractCodeRef(a); codeRef != nil {
			catalogID = codeRef.CatalogID
			versionID = codeRef.CatalogVersionID
		}

		updated := a.UpdatedAt.UTC().Format(timestampFormat)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", a.ID, a.Name, a.Status, catalogID, versionID, updated)
	}

	w.Flush()
}
