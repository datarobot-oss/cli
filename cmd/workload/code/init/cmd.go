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
	"errors"
	"fmt"
	"net/http"

	"github.com/datarobot/cli/cmd/workload/code/internal/dirprompt"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/spf13/cobra"
)

// Test seam: cmd_test.go reassigns this to stub the HTTP call.
var getArtifactFn = workload.GetArtifact

func init() {
	// --yes is read directly from cobra; only the env var binds to viper
	_ = viperx.BindEnv("yes", "DATAROBOT_CLI_NON_INTERACTIVE")
}

func Cmd() *cobra.Command {
	var outputFormat workload.OutputFormat

	c := &cobra.Command{
		Use:          "init [<artifact-id>]",
		Short:        "Link a project directory to an existing workload artifact.",
		SilenceUsage: true,
		Long: `Link a local project directory to an existing DataRobot workload
artifact in your deployment infrastructure.

This command creates a '.wapi/' state directory at the project root and
records which artifact, catalog, and version the directory is bound to.
Subsequent 'sync' invocations use this state to push local edits and
pull remote changes.

The artifact must already exist before running 'init'. Create it via
'dr workload artifact create' or in the DataRobot UI.

By default, output is human-readable. Use --output-format json for
machine-parseable output.

Example:
  dr workload code init art-abc-123                  # interactive: prompt for dir
  dr workload code init art-abc-123 --yes            # non-interactive: cwd
  dr workload code init art-abc-123 --dir ./service  # link ./service
  dr workload code init art-abc-123 --yes --output-format json`,
		Args:    cobra.MaximumNArgs(1),
		PreRunE: auth.EnsureAuthenticatedE,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd, args, outputFormat)
		},
	}

	c.Flags().String("dir", "", "Project directory (default: current directory).")
	c.Flags().BoolP("yes", "y", false, "Skip interactive prompts; use defaults.")

	workload.AddOutputFlag(c, &outputFormat)

	return c
}

func runInit(cmd *cobra.Command, args []string, outputFormat workload.OutputFormat) error {
	yesFlag, _ := cmd.Flags().GetBool("yes")
	yes := yesFlag || viperx.GetBool("yes")
	tty := reader.IsStdinTerminal()
	dirFlag, _ := cmd.Flags().GetString("dir")

	dir, err := dirprompt.ResolveDir(dirFlag, yes, tty, dirprompt.AskWithDefault)
	if err != nil {
		return err
	}

	if wapi.Exists(dir) {
		return reportAlreadyLinked(dir)
	}

	artifactID, err := dirprompt.ResolveArtifactID(args, yes, tty, dirprompt.Ask)
	if err != nil {
		return err
	}

	art, err := fetchArtifact(artifactID)
	if err != nil {
		return err
	}

	codeRef := workload.ExtractCodeRef(*art)
	opts := buildInitOptions(artifactID, codeRef)

	if err := wapi.Initialize(dir, opts); err != nil {
		if errors.Is(err, wapi.ErrAlreadyLinked) {
			return reportAlreadyLinked(dir)
		}

		return err
	}

	return renderInitResult(outputFormat, newInitResult(*art, dir))
}

func fetchArtifact(artifactID string) (*workload.Artifact, error) {
	art, err := getArtifactFn(artifactID)
	if err != nil {
		var httpErr *drapi.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("artifact %s not found", artifactID)
		}

		return nil, err
	}

	if art.IsLocked() {
		return nil, errors.New("artifact is locked (immutable); cannot init on a registered artifact")
	}

	return art, nil
}

// LastSyncedVersionID stays empty so the first sync detects drift and runs
// a full three-way diff against the remote manifest.
func buildInitOptions(artifactID string, codeRef *workload.DatarobotCodeRef) wapi.InitOptions {
	opts := wapi.InitOptions{ArtifactID: artifactID}

	if codeRef != nil {
		opts.CatalogID = codeRef.CatalogID
	}

	return opts
}

func reportAlreadyLinked(dir string) error {
	cfg, lerr := wapi.LoadConfig(dir)
	if lerr != nil {
		return fmt.Errorf("project already linked but config is unreadable: %w", lerr)
	}

	printAlreadyLinked(cfg.ArtifactID, dir)

	return errors.New("init aborted: project already linked")
}
