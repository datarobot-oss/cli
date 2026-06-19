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

package versions

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/spf13/cobra"
)

// Deps holds the externally-injected collaborators for the versions
// command. Tests build a Deps with fakes and pass it to cmdWithDeps;
// production callers go through Cmd() which uses defaultDeps().
type Deps struct {
	GetArtifact func(string) (*workload.Artifact, error)
	Files       filesapi.Client
}

func defaultDeps() Deps {
	return Deps{
		GetArtifact: workload.GetArtifact,
		Files:       filesapi.New(),
	}
}

func Cmd() *cobra.Command {
	return cmdWithDeps(defaultDeps())
}

func cmdWithDeps(deps Deps) *cobra.Command {
	var outputFormat outputformat.OutputFormat

	c := &cobra.Command{
		Use:          "versions",
		Short:        "List catalog versions for the linked artifact.",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		Long: `List the catalog version history for the artifact this
project directory is linked to.

The output marks the version that the artifact's codeRef currently
points to with '*', and reports which version the local '.wapi/'
state was last synced to.

By default output is a human-readable table; use --output-format json
for machine-parseable output.

Run 'dr artifact code init <artifact-id>' first to link a project
directory to an artifact.

Example:
  dr artifact code versions
  dr artifact code versions --limit 10
  dr artifact code versions --output-format json`,
		PreRunE: auth.EnsureAuthenticatedE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			return runVersions(cmd, outputFormat, deps)
		},
	}

	outputformat.AddFlag(c, &outputFormat)

	c.Flags().String("dir", "", "Project directory (default: current directory).")
	c.Flags().Int("limit", 100, "Maximum number of versions to return.")

	telemetry.TrackWith(c, func(cmd *cobra.Command, _ []string) map[string]any {
		limit, _ := cmd.Flags().GetInt("limit")

		return map[string]any{
			"limit":         limit,
			"output_format": string(outputFormat),
		}
	})

	return c
}

func runVersions(cmd *cobra.Command, outputFormat outputformat.OutputFormat, deps Deps) error {
	dirFlag, _ := cmd.Flags().GetString("dir")
	limit, _ := cmd.Flags().GetInt("limit")

	if limit <= 0 {
		return fmt.Errorf("invalid --limit %d: must be positive", limit)
	}

	cfg, err := loadProjectConfig(dirFlag)
	if err != nil {
		return err
	}

	v, err := buildView(cfg, limit, deps)
	if err != nil {
		return err
	}

	return render(cmd.OutOrStdout(), outputFormat, v)
}

func loadProjectConfig(dirFlag string) (wapi.Config, error) {
	dir := dirFlag
	if dir == "" {
		dir = "."
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return wapi.Config{}, fmt.Errorf("resolve dir %s: %w", dir, err)
	}

	if !wapi.Exists(absDir) {
		return wapi.Config{}, errors.New("not linked to an artifact. Run 'dr artifact code init <id>' first")
	}

	cfg, err := wapi.LoadConfig(absDir)
	if err != nil {
		return wapi.Config{}, fmt.Errorf("read .wapi/config.json: %w", err)
	}

	if cfg.CatalogID == nil || *cfg.CatalogID == "" {
		return wapi.Config{}, errors.New("no code has been synced yet. Run 'dr artifact code sync' first")
	}

	return cfg, nil
}

func buildView(cfg wapi.Config, limit int, deps Deps) (view, error) {
	art, err := deps.GetArtifact(cfg.ArtifactID)
	if err != nil {
		var httpErr *drapi.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return view{}, fmt.Errorf("artifact %s not found", cfg.ArtifactID)
		}

		return view{}, fmt.Errorf("fetch artifact %s: %w", cfg.ArtifactID, err)
	}

	versions, err := deps.Files.ListVersions(*cfg.CatalogID, limit)
	if err != nil {
		var httpErr *drapi.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return view{}, fmt.Errorf("catalog %s not found", *cfg.CatalogID)
		}

		return view{}, fmt.Errorf("list versions: %w", err)
	}

	currentVersionID := ""
	if codeRef := workload.ExtractCodeRef(*art); codeRef != nil {
		currentVersionID = codeRef.CatalogVersionID
	}

	syncedVersionID := ""
	if cfg.LastSyncedVersionID != nil {
		syncedVersionID = *cfg.LastSyncedVersionID
	}

	return newView(*art, versions, currentVersionID, syncedVersionID), nil
}

func render(out io.Writer, format outputformat.OutputFormat, v view) error {
	if format == outputformat.OutputFormatJSON {
		return renderJSON(out, v)
	}

	renderText(out, v)

	return nil
}
