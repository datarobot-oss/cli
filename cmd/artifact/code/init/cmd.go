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

	"github.com/datarobot/cli/cmd/artifact/code/internal/dirprompt"
	"github.com/datarobot/cli/cmd/artifact/code/internal/format"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/spf13/cobra"
)

// Test seam: cmd_test.go reassigns this to stub the HTTP call.
var getArtifactFn = workload.GetArtifact

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	c := &cobra.Command{
		Use:          "init [<artifact-id>]",
		Short:        "Link a project directory to an existing artifact.",
		SilenceUsage: true,
		Long: `Link a local project directory to an existing DataRobot
artifact in your deployment infrastructure.

This command creates a '.datarobot/workload/' state directory at the project
root and records which artifact, catalog, and version the directory is
bound to. Subsequent 'sync' invocations use this state to push local edits
and pull remote changes.

The artifact must already exist before running 'init'. Create it via
'dr artifact create' or in the DataRobot UI.

By default, output is human-readable. Use --output-format json for
machine-parseable output.

Example:
  dr artifact code init art-abc-123                  # interactive: prompt for dir
  dr artifact code init art-abc-123 --yes            # non-interactive: cwd
  dr artifact code init art-abc-123 --dir ./service  # link ./service
  dr artifact code init art-abc-123 --yes --output-format json`,
		Args:    cobra.MaximumNArgs(1),
		PreRunE: auth.EnsureAuthenticatedE,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			return runInit(cmd, args, outputFormat)
		},
	}

	outputformat.AddFlag(c, &outputFormat)

	c.Flags().String("dir", "", "Project directory (default: current directory).")
	c.Flags().BoolP(cli.YesFlagName, "y", false, "Skip interactive prompts; use defaults.")

	// Bind only the env var (DATAROBOT_CLI_NON_INTERACTIVE) to viper. The --yes
	// flag itself is read directly from cmd.Flags() in runInit so an explicit
	// --yes does not leak into viper.AllSettings() and persist to drconfig.yaml.
	_ = viperx.BindEnv(cli.YesFlagName, "DATAROBOT_CLI_NON_INTERACTIVE")

	telemetry.TrackWith(c, func(cmd *cobra.Command, args []string) map[string]any {
		yes := cli.IsNonInteractive(cmd)

		return map[string]any{
			"artifact_id":   telemetry.FirstArg(args),
			"yes":           yes,
			"output_format": string(outputFormat),
		}
	})

	return c
}

func runInit(cmd *cobra.Command, args []string, outputFormat outputformat.OutputFormat) error {
	yes := cli.IsNonInteractive(cmd)
	dirFlag, _ := cmd.Flags().GetString("dir")

	dir, err := dirprompt.ResolveDir(dirFlag, yes, dirprompt.AskWithDefault)
	if err != nil {
		return err
	}

	format.StateNotice(cmd.ErrOrStderr(), wapi.EnsureMigrated(dir))

	if wapi.Exists(dir) {
		return reportAlreadyLinked(cmd, dir, outputFormat)
	}

	artifactID, err := dirprompt.ResolveArtifactID(args, yes, dirprompt.Ask)
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
			return reportAlreadyLinked(cmd, dir, outputFormat)
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

// reportAlreadyLinked handles the already-linked branch: the project is
// already linked and the user tried to init again. It fetches the linked
// artifact to determine health and branches:
//   - Corrupt config (unreadable linked state): report unreadable, remedy
//     names doctor --fix, never deletion.
//   - Gone (404) or catalog mismatch: interactive → offer to relink in place;
//     non-interactive → print guidance naming doctor --relink.
//   - Healthy (or non-404 error — can't determine): keep abort behavior,
//     point to doctor for diagnosis. No delete advice anywhere.
//
// JSON mode: the abort emits a single JSON object on stdout
// {status:error, error:already-linked, artifactId:<id|null>, remedy:<guidance>}
// with human text on stderr, exit 1.
func reportAlreadyLinked(cmd *cobra.Command, dir string, outputFormat outputformat.OutputFormat) error {
	stderr := cmd.ErrOrStderr()

	cfg, err := wapi.LoadConfig(dir)
	if err != nil {
		// Corrupt config: cannot read the linked artifact id. Do NOT fetch;
		// report unreadable, remedy names doctor --fix, never deletion.
		const remedy = "dr artifact code doctor --fix"

		if outputFormat == outputformat.OutputFormatJSON {
			renderAlreadyLinkedJSON(cmd.OutOrStdout(), nil, remedy)

			fmt.Fprintln(stderr, "Project is already linked but the config is unreadable.")
			fmt.Fprintln(stderr, "Run 'dr artifact code doctor --fix' to repair the config.")

			cmd.SilenceErrors = true

			return cli.ErrSilent
		}

		printCorruptConfig(cmd.OutOrStdout(), dir)

		return errors.New("init aborted: project already linked (config unreadable)")
	}

	// Fetch the linked artifact to determine health.
	art, fetchErr := getArtifactFn(cfg.ArtifactID)

	gone := isNotFound(fetchErr)

	mismatch := false
	if fetchErr == nil && art != nil {
		mismatch = isCatalogMismatch(cfg.CatalogID, art)
	}

	if gone || mismatch {
		return handleGoneOrMismatch(cmd, dir, cfg, outputFormat, gone)
	}

	// Healthy (or non-404 error — can't determine, treat as healthy).
	const remedy = "dr artifact code doctor"

	if outputFormat == outputformat.OutputFormatJSON {
		artifactID := cfg.ArtifactID

		renderAlreadyLinkedJSON(cmd.OutOrStdout(), &artifactID, remedy)

		printAlreadyLinkedHealthy(stderr, cfg.ArtifactID, dir)

		cmd.SilenceErrors = true

		return cli.ErrSilent
	}

	printAlreadyLinkedHealthy(cmd.OutOrStdout(), cfg.ArtifactID, dir)

	return errors.New("init aborted: project already linked")
}

// isNotFound reports whether err is the API's 404 (possibly wrapped).
func isNotFound(err error) bool {
	var httpErr *drapi.HTTPError

	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound
}

// isCatalogMismatch reports whether the locally pinned catalog id no longer
// matches the artifact's codeRef. The anchor-on-local rule applies: a
// nil/empty local pin means "never synced from here" and is always OK.
func isCatalogMismatch(localCatalogID *string, art *workload.Artifact) bool {
	if localCatalogID == nil || *localCatalogID == "" {
		return false
	}

	codeRef := workload.ExtractCodeRef(*art)
	if codeRef == nil || codeRef.CatalogID == "" {
		return true // local pin set, remote absent/empty
	}

	return *localCatalogID != codeRef.CatalogID
}
