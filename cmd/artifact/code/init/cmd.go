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
	c.Flags().Bool("force", false,
		"Re-point an already-linked directory at this artifact instead of refusing. The catalog and last "+
			"synced version are taken from the artifact named here, so the next sync uploads what it lacks.")

	// Bind only the env var (DATAROBOT_CLI_NON_INTERACTIVE) to viper. The --yes
	// flag itself is read directly from cmd.Flags() in runInit so an explicit
	// --yes does not leak into viper.AllSettings() and persist to drconfig.yaml.
	_ = viperx.BindEnv(cli.YesFlagName, "DATAROBOT_CLI_NON_INTERACTIVE")

	telemetry.TrackWith(c, func(cmd *cobra.Command, args []string) map[string]any {
		yes := cli.IsNonInteractive(cmd)
		force, _ := cmd.Flags().GetBool("force")

		return map[string]any{
			"artifact_id":   telemetry.FirstArg(args),
			"yes":           yes,
			"force":         force,
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

	force, _ := cmd.Flags().GetBool("force")

	linked := wapi.Exists(dir)
	if linked && !force {
		return reportAlreadyLinked(dir)
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

	// Re-pointing and linking are one command because they are one question:
	// which artifact does this directory push to. Splitting them would leave
	// --force to be discovered separately from the thing it fixes, which is how
	// "delete the state directory" became the answer people found first.
	if linked {
		return relink(dir, *art, opts, outputFormat)
	}

	if err := wapi.Initialize(dir, opts); err != nil {
		if errors.Is(err, wapi.ErrAlreadyLinked) {
			return reportAlreadyLinked(dir)
		}

		return err
	}

	return renderInitResult(outputFormat, newInitResult(*art, dir))
}

// relink moves an existing link, naming where it came from.
//
// The previous artifact is read before the write, because after it there is
// nothing left that remembers: a user who re-pointed the wrong directory has
// the id they need to put it back only if this line printed it.
func relink(dir string, art workload.Artifact, opts wapi.InitOptions, outputFormat outputformat.OutputFormat) error {
	previous := ""
	if cfg, err := wapi.LoadConfig(dir); err == nil {
		previous = cfg.ArtifactID
	}

	// Re-pointing at the artifact already linked is not a re-point, and doing
	// it anyway is pure loss rather than a harmless rewrite. Relink rebuilds
	// the sync baseline from the artifact named, which is correct when that is
	// a different one and destructive when it is not: with BASE emptied and
	// lastSyncedVersionId cleared, the next sync sees a file that exists on
	// both sides with different bytes as an ADD_CONFLICT rather than a plain
	// local edit, renames it to <path>.LOCAL.<timestamp>, and downloads the
	// remote copy over it. The same edit uploads cleanly against the baseline
	// this call would have discarded.
	//
	// So the answer being already correct makes this a no-op, not an error:
	// --force says do not refuse, and the end state it asks for is the one the
	// directory is in.
	if previous == opts.ArtifactID {
		if outputFormat == outputformat.OutputFormatJSON {
			return renderInitResult(outputFormat, newInitResult(art, dir))
		}

		printLinkUnchanged(opts.ArtifactID, dir)

		return nil
	}

	if err := wapi.Relink(dir, opts); err != nil {
		return err
	}

	// The JSON envelope is the shape a fresh link produces plus the one thing
	// only a relink has: the artifact it came from. A caller parsing this is
	// asking which artifact the directory pushes to, and the id it stopped
	// pushing to is the undo — the same id the text path prints, which is why
	// it cannot be the text path's alone.
	if outputFormat == outputformat.OutputFormatJSON {
		result := newInitResult(art, dir)
		if previous != "" {
			result.PreviousArtifactID = &previous
		}

		return renderInitResult(outputFormat, result)
	}

	printRelinked(previous, opts.ArtifactID, dir)

	return nil
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
