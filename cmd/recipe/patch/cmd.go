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

package patch

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/datarobot/cli/cmd/recipe/internal/filearg"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/countflags"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/llm"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		outputFormat   outputformat.OutputFormat
		fromFile       string
		name           string
		description    string
		descriptorFile string
		ifVersion      int
	)

	cmd := &cobra.Command{
		Use:   "patch <recipe-id> [<file>]",
		Short: "Rename or describe a recipe, or replace its content.",
		Long: `Change a recipe's name or description, or replace its content.

--name and --description change metadata and make no new version. A file
(positional or --from-file) replaces the content and makes a new version; it
must name the version it replaces with --if-version, so a change made by
someone else in the meantime is not overwritten (the server answers 409).
--descriptor-file is accepted only together with a new payload.

The edit loop:
  dr recipe get <id> --payload > recipe.yaml
  # edit recipe.yaml
  dr recipe patch <id> recipe.yaml --if-version <N>

Only the recipe's creator or a tenant admin can patch it; anyone else can
'dr recipe copy' it.

Example:
  dr recipe patch 6a1f0c2e9b7d4e3f2a1b0c9d --name kimi-k3-prod
  dr recipe patch 6a1f0c2e9b7d4e3f2a1b0c9d recipe.yaml --if-version 3`,
		Args:         cobra.RangeArgs(1, 2),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			req, err := buildRequest(cmd, args[1:], fromFile, name, description, descriptorFile, ifVersion)
			if err != nil {
				return err
			}

			r, err := llm.PatchRecipe(args[0], req)
			if err != nil {
				return explainPatchError(err, args[0])
			}

			if err := llm.RenderRecipe(cmd.OutOrStdout(), outputFormat, *r); err != nil {
				return err
			}

			if r.Status == llm.StatusInvalid {
				return fmt.Errorf("recipe %s is now invalid (version %d) and cannot be deployed", r.ID, r.Version)
			}

			return nil
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	filearg.Add(cmd, &fromFile)
	cmd.Flags().StringVar(&name, "name", "", "New name")
	cmd.Flags().StringVar(&description, "description", "", "New description")
	cmd.Flags().StringVar(&descriptorFile, "descriptor-file", "", "New descriptor (JSON or YAML), only with a new payload")
	cmd.Flags().Var(countflags.PositiveInt(&ifVersion, 0), "if-version", "The version a new payload replaces (required with a payload)")

	telemetry.TrackWith(cmd, func(c *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"name":          c.Flags().Changed("name"),
			"description":   c.Flags().Changed("description"),
			"payload":       len(args) > 1 || fromFile != "",
			"output_format": string(outputFormat),
		}
	})

	return cmd
}

func buildRequest(cmd *cobra.Command, positional []string, fromFile, name, description, descriptorFile string, ifVersion int) (llm.PatchRequest, error) {
	var req llm.PatchRequest

	if cmd.Flags().Changed("name") {
		req.Name = &name
	}

	if cmd.Flags().Changed("description") {
		req.Description = &description
	}

	if cmd.Flags().Changed("if-version") {
		req.IfVersion = &ifVersion
	}

	path, err := filearg.Resolve(positional, fromFile, false)
	if err != nil {
		return req, err
	}

	if path != "" {
		file, err := llm.ReadRecipeFile(path)
		if err != nil {
			return req, err
		}

		req.Payload = file.Payload
		req.Descriptor = file.Descriptor
	}

	if descriptorFile != "" {
		d, err := workload.ReadSpecFile(descriptorFile)
		if err != nil {
			return req, err
		}

		req.Descriptor = d
	}

	return req, nil
}

func explainPatchError(err error, id string) error {
	var httpErr *drapi.HTTPError
	if !errors.As(err, &httpErr) {
		return err
	}

	switch httpErr.StatusCode {
	case http.StatusConflict:
		return fmt.Errorf("%w\nThe recipe changed since that version: re-read it with 'dr recipe get %s' and retry with its current version", err, id)
	case http.StatusForbidden:
		var gate *llm.GateError
		if errors.As(err, &gate) {
			return err
		}

		return fmt.Errorf("%w\nOnly the recipe's creator or a tenant admin can patch it; 'dr recipe copy %s --name <new-name>' makes your own copy", err, id)
	default:
		return err
	}
}
