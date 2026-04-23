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

package create

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat string

	var specFile string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a workload artifact.",
		Long: `Create a workload artifact in your DataRobot deployment infrastructure.

This command reads a JSON spec from a file and POSTs it to the Workload API.
The created artifact is returned and shown.

By default, output is human-readable. Use --output json for machine-parseable output.

Spec file format (minimal valid example):

  {
    "name": "my-agent",
    "spec": {
      "containerGroups": [{
        "containers": [{
          "imageUri": "nginx:latest",
          "port": 8080,
          "resourceRequest": {"cpu": 1, "memory": 536870912}
        }]
      }]
    }
  }

Required fields: name, spec.containerGroups (>=1), and at least one container
per group. Optional top-level field: description. Optional container fields:
imageUri, port, resourceRequest{cpu, memory}, codeRef. Unknown fields are
rejected before the request is sent.

Example:
  dr workload artifact create --spec-file spec.json
  dr workload artifact create --spec-file spec.json --output json`,
		Args:    cobra.NoArgs,
		PreRunE: auth.EnsureAuthenticatedE,
		RunE: func(_ *cobra.Command, _ []string) error {
			if outputFormat != "" && outputFormat != "json" {
				return fmt.Errorf("invalid output format: %s (supported: json)", outputFormat)
			}

			if specFile == "" {
				return errors.New("--spec-file is required")
			}

			payload, err := readSpecFile(specFile)
			if err != nil {
				return err
			}

			if err := workload.ValidateCreateRequest(payload); err != nil {
				return err
			}

			artifact, err := workload.CreateArtifact(payload)
			if err != nil {
				return err
			}

			if outputFormat == "json" {
				return printJSON(*artifact)
			}

			printHuman(*artifact)

			return nil
		},
	}

	cmd.Flags().StringVar(&outputFormat, "output", "", "Output format (json)")
	cmd.Flags().StringVar(&specFile, "spec-file", "", "Path to JSON spec file (required)")

	return cmd
}

func readSpecFile(path string) (json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("file not found: %s", path)
		}

		return nil, err
	}

	var probe map[string]any

	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	return json.RawMessage(data), nil
}

func printJSON(artifact workload.Artifact) error {
	data, err := json.MarshalIndent(workload.NewArtifactOutput(artifact), "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

func printHuman(artifact workload.Artifact) {
	codeRef := workload.ExtractCodeRef(artifact)

	catalogID := "\u2014"
	versionID := "\u2014"

	if codeRef != nil {
		catalogID = codeRef.CatalogID
		versionID = codeRef.CatalogVersionID
	}

	fmt.Println(tui.BaseTextStyle.Render("ID:          " + artifact.ID))
	fmt.Println(tui.BaseTextStyle.Render("Name:        " + artifact.Name))
	fmt.Println(tui.BaseTextStyle.Render("Status:      " + artifact.Status))
	fmt.Println(tui.BaseTextStyle.Render("Catalog ID:  " + catalogID))
	fmt.Println(tui.BaseTextStyle.Render("Version ID:  " + versionID))
	fmt.Println(tui.DimStyle.Render("Created:     " + artifact.CreatedAt.UTC().Format("2006-01-02 15:04 UTC")))
	fmt.Println(tui.DimStyle.Render("Updated:     " + artifact.UpdatedAt.UTC().Format("2006-01-02 15:04 UTC")))
}
