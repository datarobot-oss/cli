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
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/spf13/cobra"
)

// Test seams.
var (
	getArtifactFn = workload.GetArtifact
	newClientFn   = filesapi.New
)

func Cmd() *cobra.Command {
	var outputFormat workload.OutputFormat

	c := &cobra.Command{
		Use:          "versions",
		Short:        "List catalog versions for the linked artifact.",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		Long: `List the catalog version history for the workload artifact this
project directory is linked to.

The output marks the version that the artifact's codeRef currently
points to with '*', and reports which version the local '.wapi/'
state was last synced to.

By default output is a human-readable table; use --output-format json
for machine-parseable output.

Run 'dr workload code init <artifact-id>' first to link a project
directory to an artifact.

Example:
  dr workload code versions
  dr workload code versions --limit 10
  dr workload code versions --output-format json`,
		PreRunE: auth.EnsureAuthenticatedE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runVersions(cmd, outputFormat)
		},
	}

	c.Flags().String("dir", "", "Project directory (default: current directory).")
	c.Flags().Int("limit", 0, "Maximum number of versions to show (0 = all).")

	workload.AddOutputFlag(c, &outputFormat)

	return c
}

func runVersions(cmd *cobra.Command, outputFormat workload.OutputFormat) error {
	dirFlag, _ := cmd.Flags().GetString("dir")
	limit, _ := cmd.Flags().GetInt("limit")

	if limit < 0 {
		return fmt.Errorf("invalid --limit %d: must be >= 0 (0 = unlimited)", limit)
	}

	cfg, err := loadProjectConfig(dirFlag)
	if err != nil {
		return err
	}

	v, err := buildView(cfg, limit)
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
		return wapi.Config{}, errors.New("not linked to an artifact. Run 'dr workload code init <id>' first")
	}

	cfg, err := wapi.LoadConfig(absDir)
	if err != nil {
		return wapi.Config{}, fmt.Errorf("read .wapi/config.json: %w", err)
	}

	if cfg.CatalogID == nil || *cfg.CatalogID == "" {
		return wapi.Config{}, errors.New("no code has been synced yet. Run 'dr workload code sync' first")
	}

	return cfg, nil
}

func buildView(cfg wapi.Config, limit int) (view, error) {
	art, err := getArtifactFn(cfg.ArtifactID)
	if err != nil {
		var httpErr *drapi.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return view{}, fmt.Errorf("artifact %s not found", cfg.ArtifactID)
		}

		return view{}, fmt.Errorf("fetch artifact %s: %w", cfg.ArtifactID, err)
	}

	versions, err := newClientFn().ListVersions(*cfg.CatalogID, limit)
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

func render(out io.Writer, format workload.OutputFormat, v view) error {
	if format == workload.OutputFormatJSON {
		return renderJSON(out, v)
	}

	renderText(out, v)

	return nil
}
