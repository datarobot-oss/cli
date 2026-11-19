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
	"io"
	"net/http"
	"strings"

	"github.com/datarobot/cli/cmd/artifact/code/internal/dirprompt"
	"github.com/datarobot/cli/cmd/artifact/code/internal/format"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	wldoctor "github.com/datarobot/cli/internal/workload/doctor"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/datarobot/cli/internal/workload/wizard"
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

With --with-manifest, init also writes the .datarobot.yaml that 'dr workload up'
deploys from, taken from the running definition of the workload that runs the
artifact. It is for moving a workload to another install: run init and sync or
checkout against the source (--profile), then 'dr workload up' against the
target. The workload is the one whose artifact is exactly the one being linked.
When no workload runs it, init links the directory and writes no file; when
several do (a locked artifact can back many), init stops and lists them, and
--workload-id picks one. The file binds no workload, because a workload id
belongs to the install that issued it, and every credential reference names
PLACEHOLDER, because credential ids do too and the platform never returns a
stored value. Replace each with a credential id from the target install before
deploying. An existing .datarobot.yaml is left as it is.

By default, output is human-readable. Use --output-format json for
machine-parseable output.

Example:
  dr artifact code init art-abc-123                  # interactive: prompt for dir
  dr artifact code init art-abc-123 --yes            # non-interactive: cwd
  dr artifact code init art-abc-123 --dir ./service  # link ./service
  dr artifact code init art-abc-123 --yes --output-format json
  dr artifact code init art-abc-123 --with-manifest
  dr artifact code init art-abc-123 --with-manifest --workload-id 68b0c1d2e3f4a5b6c7d8e9f0`,
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
	c.Flags().Bool("with-manifest", false,
		"Also write .datarobot.yaml from the running definition of the workload that runs the artifact, "+
			"unbound and with credential references left to fill in, so the code can be deployed to another install.")
	c.Flags().String("workload-id", "",
		"With --with-manifest, take the definition from this workload instead of looking up the one that runs the artifact.")

	// Bind only the env var (DATAROBOT_CLI_NON_INTERACTIVE) to viper. The --yes
	// flag itself is read directly from cmd.Flags() in runInit so an explicit
	// --yes does not leak into viper.AllSettings() and persist to drconfig.yaml.
	_ = viperx.BindEnv(cli.YesFlagName, "DATAROBOT_CLI_NON_INTERACTIVE")

	telemetry.TrackWith(c, func(cmd *cobra.Command, args []string) map[string]any {
		yes := cli.IsNonInteractive(cmd)

		return map[string]any{
			"artifact_id":   telemetry.FirstArg(args),
			"yes":           yes,
			"with_manifest": cmd.Flags().Changed("with-manifest"),
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
		return reportAlreadyLinked(cmd, dir, givenArtifactID(args), outputFormat)
	}

	artifactID, err := dirprompt.ResolveArtifactID(args, yes, dirprompt.Ask)
	if err != nil {
		return err
	}

	art, err := fetchArtifact(artifactID)
	if err != nil {
		return err
	}

	// Before the link, so a workload id that does not resolve fails with
	// nothing changed.
	portable, err := prepareManifest(cmd, art)
	if err != nil {
		return err
	}

	codeRef := workload.ExtractCodeRef(*art)
	opts := buildInitOptions(artifactID, codeRef)

	if err := wapi.Initialize(dir, opts); err != nil {
		if errors.Is(err, wapi.ErrAlreadyLinked) {
			return reportAlreadyLinked(cmd, dir, artifactID, outputFormat)
		}

		return err
	}

	return finishInit(cmd, dir, *art, portable, outputFormat)
}

// finishInit writes the portable manifest, when one was asked for, and
// reports the link.
func finishInit(
	cmd *cobra.Command, dir string, art workload.Artifact, portable *wizard.PortableManifest,
	outputFormat outputformat.OutputFormat,
) error {
	result := newInitResult(art, dir)

	if portable != nil {
		if err := writeManifest(cmd.ErrOrStderr(), dir, *portable, &result); err != nil {
			return err
		}
	}

	if portable != nil {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"To deploy this to another install, delete %s first: it names this install's artifact.\n",
			wapi.Dir(dir))
	}

	return renderInitResult(cmd.ErrOrStderr(), outputFormat, result)
}

// prepareManifest finds the workload for --with-manifest and renders its
// definition for another install. It returns nil when no manifest is asked
// for, or when the artifact has no workload to take one from.
func prepareManifest(cmd *cobra.Command, art *workload.Artifact) (*wizard.PortableManifest, error) {
	withManifest, _ := cmd.Flags().GetBool("with-manifest")
	workloadID, _ := cmd.Flags().GetString("workload-id")

	if workloadID != "" && !withManifest {
		return nil, errors.New("--workload-id only applies with --with-manifest")
	}

	if !withManifest {
		return nil, nil //nolint:nilnil // no manifest asked for is not an error
	}

	if workloadID == "" {
		found, err := workloadFor(cmd.ErrOrStderr(), art)
		if err != nil || found == "" {
			return nil, err
		}

		workloadID = found
	}

	portable, err := wizard.PreparePortable(workloadID, art.ArtifactRepositoryID, cmd.ErrOrStderr())
	if err != nil {
		return nil, err
	}

	return &portable, nil
}

// workloadFor is the workload that runs the artifact, "" when none does. Several
// is an error that names them: choosing silently would write the wrong file.
func workloadFor(stderr io.Writer, art *workload.Artifact) (string, error) {
	found, err := wizard.WorkloadsRunning(art.ID)
	if err != nil {
		return "", err
	}

	switch len(found) {
	case 0:
		fmt.Fprintf(stderr, "No workload runs artifact %s, so no %s was written. "+
			"To take one from a workload that runs another version, pass --workload-id.\n",
			art.ID, manifest.FileName)

		return "", nil
	case 1:
		return found[0].ID, nil
	}

	lines := make([]string, 0, len(found))
	for _, w := range found {
		lines = append(lines, fmt.Sprintf("  %s  %s (%s)", w.ID, w.Name, w.Status))
	}

	return "", fmt.Errorf("%d workloads run artifact %s; pass --workload-id to choose one:\n%s",
		len(found), art.ID, strings.Join(lines, "\n"))
}

// writeManifest puts the portable manifest beside the link. The link is
// already made by now, so a failure says so rather than reading as if init
// did nothing. A manifest that is already there is the user's: it is left
// alone and said so, which is not a failure.
func writeManifest(stderr io.Writer, dir string, portable wizard.PortableManifest, result *initResult) error {
	path, err := portable.Write(dir)
	if errors.Is(err, manifest.ErrExists) {
		fmt.Fprintf(stderr, "%s already exists and was left as it is; edit it directly or delete it and run init with --with-manifest again.\n",
			wizard.ShortPath(path))

		return nil
	}

	if err != nil {
		return fmt.Errorf("linked the artifact, but could not write the manifest: %w", err)
	}

	pending := portable.CredentialsPending
	result.ManifestPath = path
	result.ManifestCredentialsPending = &pending

	fmt.Fprintf(stderr, "✓ Wrote %s from workload %s (%s), with no workload binding.\n",
		wizard.ShortPath(path), portable.Name, portable.WorkloadID)

	if pending > 0 {
		fmt.Fprintf(stderr, "  %d credential %s name %s: replace each with a credential id from the install you deploy to.\n",
			pending, wizard.Plural(pending, "reference", "references"), manifest.CredentialPlaceholder)
	}

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

	return art, nil
}

// givenArtifactID is the positional id, if init was called with one, so a
// remedy can name it before anything resolves it.
func givenArtifactID(args []string) string {
	if len(args) == 0 {
		return ""
	}

	return args[0]
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
//     names doctor --relink with the id init was given, never deletion.
//   - Gone (404) or catalog mismatch: interactive → offer to relink in place;
//     non-interactive → print guidance naming doctor --relink.
//   - Healthy (or non-404 error — can't determine): keep abort behavior,
//     point to doctor for diagnosis. No delete advice anywhere.
//
// JSON mode: the abort emits a single JSON object on stdout
// {status:error, error:already-linked, artifactId:<id|null>, remedy:<guidance>}
// with human text on stderr, exit 1.
func reportAlreadyLinked(cmd *cobra.Command, dir, givenID string, outputFormat outputformat.OutputFormat) error {
	stderr := cmd.ErrOrStderr()

	cfg, err := wapi.LoadConfig(dir)
	if err != nil {
		// Corrupt config: cannot read the linked artifact id. Do NOT fetch;
		// report unreadable, remedy names doctor --relink, never deletion.
		// The underlying LoadConfig error is wrapped so the user sees the
		// root cause (e.g. JSON parse error), and the config path is included
		// in both text and JSON stderr so the user knows which file is bad.
		configPath := wapi.ConfigPath(dir)

		wrappedErr := fmt.Errorf("init aborted: project already linked (config unreadable at %s): %w", configPath, err)

		if outputFormat == outputformat.OutputFormatJSON {
			renderAlreadyLinkedJSON(cmd.OutOrStdout(), nil, relinkRemedy(dir, givenID))

			fmt.Fprintf(stderr, "Project is already linked but the config at %s is unreadable: %v\n", configPath, err)
			fmt.Fprintln(stderr, corruptConfigRemedy(dir, givenID))

			cmd.SilenceErrors = true

			return cli.ErrSilent
		}

		printCorruptConfig(cmd.OutOrStdout(), dir, givenID)

		return wrappedErr
	}

	// Fetch the linked artifact to determine health.
	art, fetchErr := getArtifactFn(cfg.ArtifactID)

	gone := wldoctor.IsNotFound(fetchErr)

	mismatch := false
	if fetchErr == nil && art != nil {
		mismatch = wldoctor.IsCatalogMismatch(cfg.CatalogID, art)
	}

	if gone || mismatch {
		return handleGoneOrMismatch(cmd, dir, cfg, givenID, outputFormat, gone)
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
