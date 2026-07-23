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

// Package up implements `dr workload up`: a fused deploy verb driven by
// .datarobot/workload.yaml. It creates the artifact and links the directory if
// needed, then syncs -> builds -> optionally locks -> creates the workload ->
// waits -> prints the URL, so `dr workload config` + `dr workload up` is the
// whole flow.
package up

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/cmd/internal/pollflags"
	"github.com/datarobot/cli/cmd/workload/internal/wlprompt"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/datarobot/cli/internal/workload/wlconfig"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

// Poll cadence defaults. The workload wait follows Marcus's diagram (5s / 10m,
// overridable via --poll-interval/--poll-timeout). The build wait keeps the
// build-oriented defaults (2s / 30m) since container builds run far longer.
const (
	workloadPollInterval = 5 * time.Second
	workloadPollTimeout  = 10 * time.Minute
)

// Spec conventions. The group label is free; the container name must match
// between the artifact spec and the workload runtime override.
const (
	defaultContainerGroup = "default"
	primaryContainerName  = "primary"
)

// Readiness probe defaults for a created artifact (seconds).
const (
	probeInitialDelay     = 10
	probePeriod           = 10
	probeTimeout          = 5
	probeFailureThreshold = 6
)

// Test seams: cmd_test.go reassigns these to drive the orchestrator without a
// live server or a real sync.
var (
	getArtifactFn     = workload.GetArtifact
	createArtifactFn  = workload.CreateArtifact
	resolveEEFn       = workload.ResolveExecutionEnvironment
	initProjectFn     = wapi.Initialize
	runSyncFn         = defaultRunSync
	triggerBuildFn    = workload.TriggerArtifactBuild
	waitForBuildFn    = workload.WaitForBuild
	lockArtifactFn    = workload.LockArtifact
	createWorkloadFn  = workload.CreateWorkload
	getWorkloadFn     = workload.GetWorkload
	listWorkloadsFn   = workload.ListWorkloads
	startWorkloadFn   = workload.StartWorkload
	waitForWorkloadFn = workload.WaitForWorkload
)

// adoptListLimit caps the workload listing used to find the workload already
// backing the linked draft artifact after a create 409.
const adoptListLimit = 100

// upResult is the stable JSON shape emitted by --output-format json.
type upResult struct {
	WorkloadID string `json:"workloadId"`
	Status     string `json:"status"`
	Endpoint   string `json:"endpoint"`
}

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	var poll pollflags.Set

	cmd := &cobra.Command{
		Use:   "up",
		Short: "Build and deploy the workload described by .datarobot/workload.yaml.",
		Long: `Turn .datarobot/workload.yaml into a running workload with one command.

For the code-build modes (your Dockerfile, or a DataRobot-generated build),
'up' creates the artifact and links this directory if needed, syncs your
code, builds the image, optionally locks the artifact (--lock), creates the
workload with the manifest's resources, then blocks until it is running and
prints the endpoint URL as the final stdout line.

For a pre-built image manifest (build.image set), 'up' skips sync and build
entirely: it creates the workload with the artifact inline in one call.

Run 'dr workload config' first to generate the manifest. On a re-run, 'up'
reaches the recorded workload and ensures it is running.

By default 'up' blocks with progress; --detach returns immediately after
the deploy is requested.

Example:
  dr workload up
  dr workload up --lock
  dr workload up --detach
  dr workload up --yes --output-format json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			return runUp(cmd, outputFormat, poll)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().String("dir", "", "Project directory (default: current directory).")
	cmd.Flags().BoolP("yes", "y", false, "Skip interactive prompts; use defaults.")
	cmd.Flags().Bool("detach", false, "Return immediately after requesting the deploy; do not wait for running.")
	cmd.Flags().Bool("lock", false, "Lock the artifact (immutable, versioned) before deploying.")

	// Register only the poll cadence knobs (hidden). `up` blocks by default and
	// is toggled by --detach, so pollflags' visible --wait flag is deliberately
	// not used here: it would be dead and contradict --detach.
	cmd.Flags().Var(pollflags.PositiveDuration(&poll.Interval, workloadPollInterval), "poll-interval", "Interval between workload status polls.")
	cmd.Flags().Var(pollflags.PositiveDuration(&poll.Timeout, workloadPollTimeout), "poll-timeout", "Maximum time to wait for the workload to become running.")
	_ = cmd.Flags().MarkHidden("poll-interval")
	_ = cmd.Flags().MarkHidden("poll-timeout")

	_ = viperx.BindEnv("yes", "DATAROBOT_CLI_NON_INTERACTIVE")

	telemetry.TrackWith(cmd, func(cmd *cobra.Command, _ []string) map[string]any {
		yesFlag, _ := cmd.Flags().GetBool("yes")
		detach, _ := cmd.Flags().GetBool("detach")
		lock, _ := cmd.Flags().GetBool("lock")

		return map[string]any{
			"yes":           yesFlag || viperx.GetBool("yes"),
			"detach":        detach,
			"lock":          lock,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}

// upFlags is the parsed view of the command flags, folding the
// DATAROBOT_CLI_NON_INTERACTIVE env override into Yes.
type upFlags struct {
	Yes    bool
	Detach bool
	Lock   bool
}

func parseUpFlags(cmd *cobra.Command) upFlags {
	yesFlag, _ := cmd.Flags().GetBool("yes")
	detach, _ := cmd.Flags().GetBool("detach")
	lock, _ := cmd.Flags().GetBool("lock")

	return upFlags{
		Yes:    yesFlag || viperx.GetBool("yes"),
		Detach: detach,
		Lock:   lock,
	}
}

func runUp(cmd *cobra.Command, outputFormat outputformat.OutputFormat, poll pollflags.Set) error {
	flags := parseUpFlags(cmd)

	dirFlag, _ := cmd.Flags().GetString("dir")

	dir, err := wlprompt.ResolveDir(dirFlag, flags.Yes)
	if err != nil {
		return err
	}

	projectDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", dir, err)
	}

	cfg, err := wlconfig.Load(projectDir)
	if err != nil {
		if errors.Is(err, wlconfig.ErrNotConfigured) {
			return fmt.Errorf("no %s found; run `dr workload config` first", wlconfig.Path(projectDir))
		}

		return err
	}

	return orchestrate(cmd, projectDir, &cfg, flags, poll, outputFormat)
}

// orchestrate runs ensureLinked -> sync -> build -> deploy -> optional lock ->
// wait -> render, keeping runUp's flag/config resolution separate from the flow.
// Pre-built image manifests take the short path: no code, no sync, no build.
func orchestrate( //nolint: cyclop
	cmd *cobra.Command,
	projectDir string,
	cfg *wlconfig.Config,
	flags upFlags,
	poll pollflags.Set,
	outputFormat outputformat.OutputFormat,
) error {
	if cfg.BuildMode() == wlconfig.ModeImage {
		return orchestrateImage(cmd, projectDir, cfg, flags, poll, outputFormat)
	}

	// Phase: prepare artifact ------------------------------------------------
	var (
		artifactID string
		art        *workload.Artifact
		linkDetail string
	)

	start := time.Now()

	err := tui.RunWithSpinner("Preparing artifact", func() error {
		id, a, detail, e := ensureLinked(projectDir, cfg)
		artifactID, art, linkDetail = id, a, detail

		return e
	})
	if err != nil {
		return err
	}

	if linkDetail != "" {
		phaseComplete(cmd, linkDetail, time.Since(start))
	} else {
		phaseComplete(cmd, "Artifact ready", time.Since(start))
	}

	// Phase: sync code -------------------------------------------------------
	var syncResult *sync.Result

	start = time.Now()

	err = tui.RunWithSpinner("Syncing code", func() error {
		r, e := runSyncFn(projectDir)
		syncResult = r

		return e
	})
	if err != nil {
		return err
	}

	phaseComplete(cmd, "Code synced", time.Since(start))
	reportSyncConflicts(cmd, syncResult)

	// Phase: build image -----------------------------------------------------
	var failedBuild *workload.Build

	start = time.Now()

	err = tui.RunWithSpinner("Building image (this may take a minute)", func() error {
		b, e := buildAndWait(artifactID)
		failedBuild = b

		return e
	})
	if err != nil {
		printBuildFailure(cmd, failedBuild)

		return err
	}

	phaseComplete(cmd, "Image built", time.Since(start))

	// Phase: deploy workload -------------------------------------------------
	var (
		wl            *workload.Workload
		deployWarning string
	)

	start = time.Now()

	err = tui.RunWithSpinner("Deploying workload (this may take a minute)", func() error {
		w, warning, e := deploy(projectDir, cfg, artifactID, art)
		wl, deployWarning = w, warning

		return e
	})
	if err != nil {
		return err
	}

	phaseComplete(cmd, fmt.Sprintf("Launched workload %s", wl.ID), time.Since(start))

	if deployWarning != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "    %s\n", tui.HintStyle.Render(deployWarning))
	}

	// Phase: optional lock ---------------------------------------------------
	if flags.Lock {
		err = runPhase(cmd, "Locking artifact", func() error {
			_, e := lockArtifactFn(artifactID)

			return e
		})
		if err != nil {
			return err
		}
	}

	// Phase: wait for running ------------------------------------------------
	if !flags.Detach {
		start = time.Now()

		err = tui.RunWithSpinner("Waiting for workload to become running", func() error {
			w, e := waitForWorkloadFn(wl.ID, poll.Interval, poll.Timeout, nil)
			wl = w

			return e
		})
		if err != nil {
			return err
		}

		phaseComplete(cmd, fmt.Sprintf("Workload %s is running", wl.ID), time.Since(start))
	}

	return renderUp(cmd, outputFormat, wl, flags.Detach)
}

// orchestrateImage is the pre-built-image path (Tutorial 2): create the
// workload with the artifact inline in one call, optionally lock the artifact
// the server minted, wait, render. No sync, no build, no project link.
func orchestrateImage(
	cmd *cobra.Command,
	projectDir string,
	cfg *wlconfig.Config,
	flags upFlags,
	poll pollflags.Set,
	outputFormat outputformat.OutputFormat,
) error {
	var (
		wl            *workload.Workload
		deployWarning string
	)

	start := time.Now()

	err := tui.RunWithSpinner("Deploying workload (this may take a minute)", func() error {
		w, warning, e := deployImage(projectDir, cfg)
		wl, deployWarning = w, warning

		return e
	})
	if err != nil {
		return err
	}

	phaseComplete(cmd, fmt.Sprintf("Launched workload %s", wl.ID), time.Since(start))

	if deployWarning != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "    %s\n", tui.HintStyle.Render(deployWarning))
	}

	// The inline-create response carries the id of the draft artifact the
	// server created; locking it moves the workload into the production
	// lifecycle (Tutorial 2, step 2).
	if flags.Lock {
		err = runPhase(cmd, "Locking artifact", func() error {
			_, e := lockArtifactFn(wl.ArtifactID)

			return e
		})
		if err != nil {
			return err
		}
	}

	if !flags.Detach {
		start = time.Now()

		err = tui.RunWithSpinner("Waiting for workload to become running", func() error {
			w, e := waitForWorkloadFn(wl.ID, poll.Interval, poll.Timeout, nil)
			wl = w

			return e
		})
		if err != nil {
			return err
		}

		phaseComplete(cmd, fmt.Sprintf("Workload %s is running", wl.ID), time.Since(start))
	}

	return renderUp(cmd, outputFormat, wl, flags.Detach)
}

// deployImage creates the workload with the manifest's image inline on first
// run, or reaches the recorded workload on a re-run.
func deployImage(projectDir string, cfg *wlconfig.Config) (*workload.Workload, string, error) {
	if cfg.WorkloadID != "" {
		return reachExistingWorkload(projectDir, cfg)
	}

	name := cfg.Name
	if name == "" {
		name = filepath.Base(projectDir)
	}

	wl, err := createWorkloadFn(buildImageCreatePayload(name, cfg))
	if err != nil {
		return nil, "", err
	}

	if err := recordWorkloadID(projectDir, cfg, wl); err != nil {
		return nil, "", err
	}

	return wl, "", nil
}

// buildImageCreatePayload assembles the one-call create for image mode: the
// artifact (image, port, entrypoint, env, readiness probe) inline plus the
// runtime resources, mirroring Tutorial 2's spec.
func buildImageCreatePayload(name string, cfg *wlconfig.Config) map[string]any {
	container := map[string]any{
		"name":     primaryContainerName,
		"imageUri": cfg.Build.Image,
		"port":     cfg.Port(),
		"primary":  true,
		"readinessProbe": map[string]any{
			"path": cfg.Health(),
			"port": cfg.Port(),
		},
	}

	if len(cfg.Build.Entrypoint) > 0 {
		container["entrypoint"] = cfg.Build.Entrypoint
	}

	if env := envVarsPayload(cfg); env != nil {
		container["environmentVars"] = env
	}

	payload := map[string]any{
		"name": name,
		"artifact": map[string]any{
			"name": name + "-artifact",
			"type": "service",
			"spec": map[string]any{
				"containerGroups": []any{
					map[string]any{"name": defaultContainerGroup, "containers": []any{container}},
				},
			},
		},
		"runtime": runtimeBlock(cfg, primaryContainerName),
	}

	if cfg.Importance != "" {
		payload["importance"] = cfg.Importance
	}

	return payload
}

// envVarsPayload renders the manifest's env map as the API's environmentVars
// list, expanding ${VAR} references from the deploying shell so secrets can
// stay out of the committed manifest. Keys are sorted for determinism.
func envVarsPayload(cfg *wlconfig.Config) []any {
	if len(cfg.Env) == 0 {
		return nil
	}

	out := make([]any, 0, len(cfg.Env))

	for _, k := range slices.Sorted(maps.Keys(cfg.Env)) {
		out = append(out, map[string]any{"name": k, "value": os.Expand(cfg.Env[k], os.Getenv)})
	}

	return out
}

// ensureLinked returns the artifact to deploy, creating it and linking the
// project directory from the manifest's build settings when the directory is not
// linked yet. This is what lets `dr workload up` run with only `dr workload
// config` before it, with no manual `dr artifact create` / `code init`.
// The returned string is a human detail message (non-empty only when a new
// artifact was created).
func ensureLinked(projectDir string, cfg *wlconfig.Config) (string, *workload.Artifact, string, error) {
	if !wapi.Exists(projectDir) {
		return createAndLink(projectDir, cfg)
	}

	wcfg, err := wapi.LoadConfig(projectDir)
	if err != nil {
		return "", nil, "", err
	}

	art, err := getArtifactFn(wcfg.ArtifactID)
	if err != nil {
		return "", nil, "", err
	}

	if art.IsLocked() {
		return "", nil, "", fmt.Errorf("artifact %s is locked (immutable); `dr workload up` cannot redeploy through a locked artifact. Create a new artifact and workload to deploy changes", wcfg.ArtifactID)
	}

	return wcfg.ArtifactID, art, "", nil
}

// createAndLink creates a draft artifact from the manifest's build settings and
// links the project directory to it.
func createAndLink(projectDir string, cfg *wlconfig.Config) (string, *workload.Artifact, string, error) {
	spec, err := buildArtifactSpec(cfg)
	if err != nil {
		return "", nil, "", err
	}

	art, err := createArtifactFn(spec)
	if err != nil {
		return "", nil, "", err
	}

	if err := initProjectFn(projectDir, wapi.InitOptions{ArtifactID: art.ID}); err != nil {
		return "", nil, "", fmt.Errorf("artifact %s created but linking %s failed: %w", art.ID, projectDir, err)
	}

	detail := fmt.Sprintf("Created artifact %s and linked %s", art.ID, projectDir)

	return art.ID, art, detail, nil
}

// buildArtifactSpec builds the CreateArtifact payload from the manifest, in
// provided-Dockerfile or generated (execution-environment) mode.
func buildArtifactSpec(cfg *wlconfig.Config) (map[string]any, error) {
	dockerfile, err := dockerfileSpec(cfg)
	if err != nil {
		return nil, err
	}

	container := map[string]any{
		"name":             primaryContainerName,
		"imageUri":         "placeholder:latest",
		"primary":          true,
		"port":             cfg.Port(),
		"imageBuildConfig": map[string]any{"dockerfile": dockerfile},
		"readinessProbe": map[string]any{
			"path":                cfg.Health(),
			"port":                cfg.Port(),
			"initialDelaySeconds": probeInitialDelay,
			"periodSeconds":       probePeriod,
			"timeoutSeconds":      probeTimeout,
			"failureThreshold":    probeFailureThreshold,
			"scheme":              "HTTP",
		},
	}

	if env := envVarsPayload(cfg); env != nil {
		container["environmentVars"] = env
	}

	return map[string]any{
		"name": cfg.Name + "-artifact",
		"type": "service",
		"spec": map[string]any{
			"containerGroups": []any{
				map[string]any{"name": defaultContainerGroup, "containers": []any{container}},
			},
		},
	}, nil
}

// dockerfileSpec returns the imageBuildConfig.dockerfile block for the manifest's
// build mode, resolving the execution environment by name for generated mode.
func dockerfileSpec(cfg *wlconfig.Config) (map[string]any, error) {
	if cfg.BuildMode() == wlconfig.ModeProvided {
		return map[string]any{"source": "provided", "path": cfg.Build.Dockerfile}, nil
	}

	eeName := wlconfig.DefaultExecutionEnvironment
	if cfg.Build != nil && cfg.Build.ExecutionEnvironment != "" {
		eeName = cfg.Build.ExecutionEnvironment
	}

	eeID, eeVer, err := resolveEEFn(eeName)
	if err != nil {
		return nil, err
	}

	spec := map[string]any{
		"source":                        "generated",
		"executionEnvironmentId":        eeID,
		"executionEnvironmentVersionId": eeVer,
	}

	if cfg.Build != nil && len(cfg.Build.Entrypoint) > 0 {
		spec["entrypoint"] = cfg.Build.Entrypoint
	}

	return spec, nil
}

// reportSyncConflicts surfaces auto-resolved sync conflicts. `up` runs the sync
// engine with Yes:true (remote wins), so unlike interactive `dr artifact code
// sync` the user is not prompted; at minimum we must tell them their working
// copy was replaced and where the backups are.
func reportSyncConflicts(cmd *cobra.Command, result *sync.Result) {
	if result == nil || result.ConflictCount == 0 {
		return
	}

	fmt.Fprintf(cmd.ErrOrStderr(),
		"  %d conflicting file(s) auto-resolved (remote won); your versions were saved as:\n",
		result.ConflictCount)

	for _, copyPath := range result.ConflictCopies {
		fmt.Fprintf(cmd.ErrOrStderr(), "    %s\n", copyPath)
	}
}

// buildAndWait triggers a build for artifactID and blocks on each returned
// build until it reaches a terminal status. On failure it returns the failed
// build so the caller can print log tails after the spinner exits.
func buildAndWait(artifactID string) (*workload.Build, error) {
	resp, err := triggerBuildFn(artifactID)
	if err != nil {
		return nil, err
	}

	if len(resp.BuildIDs) == 0 {
		return nil, errors.New("no build IDs returned by server")
	}

	for _, id := range resp.BuildIDs {
		build, werr := waitForBuildFn(artifactID, id, pollflags.DefaultPollInterval, pollflags.DefaultPollTimeout, nil)
		if werr != nil {
			return build, werr
		}
	}

	return nil, nil
}

// printBuildFailure dumps the tail of a failed build's logs to stderr so the
// user does not have to run a second command to see why the build failed.
func printBuildFailure(cmd *cobra.Command, build *workload.Build) {
	if build == nil {
		return
	}

	summary, err := workload.BuildSummaryFor(build, workload.DefaultBuildLogTail)
	if err != nil || len(summary.LogTail) == 0 {
		return
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "  --- last %d log line(s) for build %s ---\n", len(summary.LogTail), build.ID)

	for _, entry := range summary.LogTail {
		fmt.Fprintf(cmd.ErrOrStderr(), "  %s\n", entry.Message)
	}
}

// deploy creates the workload on first run (recording its id back into the
// manifest for idempotent re-runs) or reaches an already-created workload.
// The returned warning string is non-empty when the workload already exists and
// cannot be updated in place.
func deploy(projectDir string, cfg *wlconfig.Config, artifactID string, art *workload.Artifact) (*workload.Workload, string, error) {
	if cfg.WorkloadID == "" {
		wl, err := createWorkload(projectDir, cfg, artifactID, art)

		return wl, "", err
	}

	return reachExistingWorkload(projectDir, cfg)
}

// createWorkload is the first-deploy path: create the workload from the linked
// artifact and record its id so re-runs are idempotent.
func createWorkload(projectDir string, cfg *wlconfig.Config, artifactID string, art *workload.Artifact) (*workload.Workload, error) {
	name := cfg.Name
	if name == "" {
		name = filepath.Base(projectDir)
	}

	wl, err := createWorkloadFn(buildCreatePayload(name, artifactID, art, cfg))
	if err != nil {
		// The server allows only one workload per draft artifact. If that is
		// what we hit, the manifest lost its workloadId (rewritten by an old
		// config, hand-edited, fresh clone): adopt the existing workload
		// instead of dead-ending on the 409.
		wl = adoptWorkloadForArtifact(artifactID, err)
		if wl == nil {
			return nil, err
		}
	}

	if err := recordWorkloadID(projectDir, cfg, wl); err != nil {
		return nil, err
	}

	return wl, nil
}

// adoptWorkloadForArtifact recovers from the one-workload-per-draft-artifact
// rule: on a create 409, find the workload already backing artifactID and
// return it so the deploy continues with it. Returns nil when the error is not
// that 409 or no backing workload is found (the caller keeps the original
// error).
func adoptWorkloadForArtifact(artifactID string, createErr error) *workload.Workload {
	var httpErr *drapi.HTTPError

	if !errors.As(createErr, &httpErr) || httpErr.StatusCode != http.StatusConflict {
		return nil
	}

	workloads, err := listWorkloadsFn(adoptListLimit, nil)
	if err != nil {
		return nil
	}

	for i := range workloads {
		if workloads[i].ArtifactID != artifactID {
			continue
		}

		return &workloads[i]
	}

	return nil
}

// recordWorkloadID writes the created workload's id back into the manifest so
// re-runs are idempotent. On failure it tells the user how to record the id by
// hand, so a blind retry does not create a duplicate.
func recordWorkloadID(projectDir string, cfg *wlconfig.Config, wl *workload.Workload) error {
	cfg.WorkloadID = wl.ID

	if err := wlconfig.Save(projectDir, *cfg); err != nil {
		return fmt.Errorf(
			"workload %s was created but recording its id in %s failed: %w\nset `workloadId: %s` in that file before re-running to avoid creating a duplicate",
			wl.ID, wlconfig.Path(projectDir), err, wl.ID)
	}

	return nil
}

// reachExistingWorkload is the re-deploy path. It brings a non-running workload
// back up and is honest that a newly built image is not auto-applied to an
// already-running workload. The returned warning is non-empty when the workload
// is already running and cannot be updated in place.
func reachExistingWorkload(projectDir string, cfg *wlconfig.Config) (*workload.Workload, string, error) {
	wl, err := getWorkloadFn(cfg.WorkloadID)
	if err != nil {
		var httpErr *drapi.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, "", fmt.Errorf(
				"workload %s (recorded in %s) no longer exists; run `dr workload config` to bind or create another",
				cfg.WorkloadID, wlconfig.Path(projectDir))
		}

		return nil, "", err
	}

	// Bring a stopped/suspended/interrupted workload back up, then re-fetch so
	// we do not report the stale pre-start status.
	if isStoppedLike(wl.Status) {
		if _, serr := startWorkloadFn(cfg.WorkloadID); serr != nil {
			return nil, "", serr
		}

		if refetched, rerr := getWorkloadFn(cfg.WorkloadID); rerr == nil {
			wl = refetched
		}

		return wl, "", nil
	}

	warning := fmt.Sprintf(
		"workload %s already exists; updating a running workload in place is not automated yet. Recreate or use a replace flow to roll out changes.",
		cfg.WorkloadID)

	return wl, warning, nil
}

// buildCreatePayload assembles the workload create request for the code-build
// modes. The server requires a resource signal (resourceAllocation here); the
// container name must match the artifact's primary container so the override
// binds to it.
func buildCreatePayload(name, artifactID string, art *workload.Artifact, cfg *wlconfig.Config) map[string]any {
	containerName := primaryContainerName

	if art != nil {
		if n := workload.PrimaryContainerName(*art); n != "" {
			containerName = n
		}
	}

	payload := map[string]any{
		"name":       name,
		"artifactId": artifactID,
		"runtime":    runtimeBlock(cfg, containerName),
	}

	if cfg != nil && cfg.Importance != "" {
		payload["importance"] = cfg.Importance
	}

	return payload
}

// runtimeBlock renders the workload runtime with the manifest's resources,
// falling back to the manifest defaults.
func runtimeBlock(cfg *wlconfig.Config, containerName string) map[string]any {
	replicas := wlconfig.DefaultReplicas
	cpu := wlconfig.DefaultCPU
	memory := wlconfig.DefaultMemory

	if cfg != nil && cfg.Runtime != nil {
		if cfg.Runtime.Replicas > 0 {
			replicas = cfg.Runtime.Replicas
		}

		if cfg.Runtime.CPU > 0 {
			cpu = cfg.Runtime.CPU
		}

		if cfg.Runtime.Memory != "" {
			memory = cfg.Runtime.Memory
		}
	}

	return map[string]any{
		"containerGroups": []any{
			map[string]any{
				"name":         defaultContainerGroup,
				"replicaCount": replicas,
				"containers": []any{
					map[string]any{
						"name":               containerName,
						"resourceAllocation": map[string]any{"cpu": cpu, "memory": memory},
					},
				},
			},
		},
	}
}

func isStoppedLike(status string) bool {
	switch status {
	case workload.WorkloadStatusStopped,
		workload.WorkloadStatusSuspended,
		workload.WorkloadStatusInterrupted:
		return true
	}

	return false
}

func renderUp(cmd *cobra.Command, outputFormat outputformat.OutputFormat, wl *workload.Workload, detached bool) error {
	// A workload in a failed terminal state is not a success, on the detach
	// path too where no wait ran to catch it: emit the record, then error.
	failed := workload.IsWorkloadErrorStatus(wl.Status)

	if outputFormat == outputformat.OutputFormatJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")

		if err := enc.Encode(upResult{WorkloadID: wl.ID, Status: wl.Status, Endpoint: wl.Endpoint}); err != nil {
			return err
		}

		if failed {
			return fmt.Errorf("workload %s is in a failed state: %s", wl.ID, wl.Status)
		}

		return nil
	}

	// For --detach the caller skipped the wait spinner, so we print status here.
	if detached {
		fmt.Fprintf(cmd.ErrOrStderr(), "  %s %s\n",
			checkStyle.Render("✓"),
			fmt.Sprintf("Workload %s is %s (not waiting)", wl.ID, wl.Status))
	}

	if failed {
		return fmt.Errorf("workload %s is in a failed state: %s", wl.ID, wl.Status)
	}

	// The bare endpoint URL is the last stdout line, matching the
	// `dr workload endpoint` contract so scripts can capture it.
	if wl.Endpoint == "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "    %s\n", tui.HintStyle.Render("No endpoint URL yet; check `dr workload status`."))

		return nil
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "\n  %s %s\n", "🚀", tui.SuccessStyle.Render("Endpoint ready:"))
	fmt.Fprintln(cmd.OutOrStdout(), wl.Endpoint)

	return nil
}

// phaseComplete prints a styled completion line with elapsed time to stderr so
// stdout stays reserved for the endpoint URL / JSON contract.
var (
	checkStyle    = lipgloss.NewStyle().Foreground(tui.GetAdaptiveColor(tui.DrGreen, tui.DrGreenDark))
	durationStyle = lipgloss.NewStyle().Foreground(tui.GetAdaptiveColor(tui.DrGray, tui.DrGrayDark))
)

func phaseComplete(cmd *cobra.Command, msg string, elapsed time.Duration) {
	dur := durationStyle.Render(fmt.Sprintf("(%s)", elapsed.Truncate(100*time.Millisecond)))
	fmt.Fprintf(cmd.ErrOrStderr(), "  %s %s %s\n", checkStyle.Render("✓"), msg, dur)
}

// runPhase wraps a function in a spinner, times it, and prints a ✓ completion
// line when done.
func runPhase(cmd *cobra.Command, label string, fn func() error) error {
	start := time.Now()

	err := tui.RunWithSpinner(label, fn)
	if err != nil {
		return err
	}

	phaseComplete(cmd, label, time.Since(start))

	return nil
}

// defaultRunSync links the sync engine to projectDir and runs a full,
// auto-resolving sync (remote wins on conflict, local copies preserved).
func defaultRunSync(projectDir string) (*sync.Result, error) {
	if !wapi.Exists(projectDir) {
		return nil, errors.New("project not linked; `dr workload up` links it automatically, so this is unexpected")
	}

	engine, err := sync.New(projectDir, sync.Options{Yes: true})
	if err != nil {
		return nil, err
	}

	defer func() { _ = engine.Close() }()

	return engine.Run()
}
