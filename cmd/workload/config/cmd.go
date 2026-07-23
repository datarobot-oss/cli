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

// Package config implements `dr workload config`: an interactive command that
// selects or names a workload and writes the committed .datarobot/workload.yaml
// that `dr workload up` consumes.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/cmd/workload/internal/wlprompt"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wlconfig"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

// workloadPickLimit caps how many existing workloads are fetched for the
// interactive picker; more than this and the user should pass --workload-id.
const workloadPickLimit = 100

// Test seams: cmd_test.go reassigns these to bypass the network and the TUI.
var (
	listWorkloadsFn   = workload.ListWorkloads
	runPickerFn       = runPicker
	askFn             = wlprompt.Ask
	isStdinTerminalFn = reader.IsStdinTerminal
	promptBuildFn     = promptBuild
)

// configResult is the stable JSON shape emitted by --output-format json.
type configResult struct {
	WorkloadID string `json:"workloadId"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	// CreateOnUp is true when no existing workload was selected, so the
	// workload is created on the first `dr workload up`.
	CreateOnUp bool `json:"createOnUp"`
}

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Configure a workload and write .datarobot/workload.yaml.",
		Long: `Interactively select an existing workload or name a new one, then
write the committed .datarobot/workload.yaml that 'dr workload up' reads.

Selecting an existing workload records its id immediately. Naming a new
workload records the name only; the workload itself is created on the
first 'dr workload up' (so no empty placeholder workload is minted).

For non-interactive use, pass --workload-id to bind an existing workload or
--name to record a new one; either makes the command non-interactive on its
own (no --yes needed). Bare --yes with neither flag is an error.

Example:
  dr workload config
  dr workload config --workload-id 68b0c1d2e3f4a5b6c7d8e9f0
  dr workload config --name my-app`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			return runConfig(cmd, outputFormat)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().String("dir", "", "Project directory (default: current directory).")
	cmd.Flags().BoolP("yes", "y", false, "Skip interactive prompts; requires --workload-id or --name.")
	cmd.Flags().String("workload-id", "", "Bind an existing workload by id (non-interactive).")
	cmd.Flags().String("name", "", "Name a new workload, created on first `dr workload up` (non-interactive).")

	// Only the env var binds to viper; --yes is read directly from cobra so it
	// never persists into drconfig.yaml.
	_ = viperx.BindEnv("yes", "DATAROBOT_CLI_NON_INTERACTIVE")

	telemetry.TrackWith(cmd, func(cmd *cobra.Command, _ []string) map[string]any {
		yesFlag, _ := cmd.Flags().GetBool("yes")

		return map[string]any{
			"yes":           yesFlag || viperx.GetBool("yes"),
			"output_format": string(outputFormat),
		}
	})

	return cmd
}

func runConfig(cmd *cobra.Command, outputFormat outputformat.OutputFormat) error {
	dirFlag, _ := cmd.Flags().GetString("dir")
	yesFlag, _ := cmd.Flags().GetBool("yes")
	yes := yesFlag || viperx.GetBool("yes")
	idFlag, _ := cmd.Flags().GetString("workload-id")
	nameFlag, _ := cmd.Flags().GetString("name")

	dir, err := wlprompt.ResolveDir(dirFlag, yes)
	if err != nil {
		return err
	}

	projectDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", dir, err)
	}

	id, name, err := selectWorkload(yes, idFlag, nameFlag)
	if err != nil {
		return err
	}

	if name == "" {
		name = filepath.Base(projectDir)
	}

	// Start from a fully-defaulted manifest so `up` can build and deploy with no
	// further input, then let an interactive user adjust the build mode.
	cfg := wlconfig.Default(name)
	cfg.WorkloadID = id

	preserveRecordedWorkloadID(cmd, projectDir, &cfg)

	if !yes && isStdinTerminalFn() {
		if err := promptBuildFn(cmd, &cfg); err != nil {
			return err
		}
	}

	if err := wlconfig.Save(projectDir, cfg); err != nil {
		return err
	}

	return renderResult(cmd, outputFormat, cfg, wlconfig.Path(projectDir))
}

// promptBuild asks the one question that can't be defaulted well, how the image
// comes to be, then the fields specific to that mode. Port and resources keep
// their defaults; the written manifest is commented so the user can edit them.
// A read error (Ctrl-C/EOF) aborts rather than silently accepting defaults.
func promptBuild(cmd *cobra.Command, cfg *wlconfig.Config) error {
	w := cmd.ErrOrStderr()
	numStyle := lipgloss.NewStyle().Foreground(tui.DrPurple).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#333333", Dark: "#CCCCCC"})

	fmt.Fprintln(w)
	fmt.Fprintln(w, tui.BaseTextStyle.Render("  Build mode:"))
	fmt.Fprintf(w, "    %s  %s\n", numStyle.Render("1"), descStyle.Render("Use your Dockerfile"))
	fmt.Fprintf(w, "    %s  %s\n", numStyle.Render("2"), descStyle.Render("DataRobot builds it (generated)"))
	fmt.Fprintf(w, "    %s  %s\n", numStyle.Render("3"), descStyle.Render("Deploy a pre-built image"))

	mode, err := wlprompt.AskWithDefault("Choose 1, 2, or 3", "1")
	if err != nil {
		return err
	}

	switch strings.TrimSpace(mode) {
	case "2":
		return promptGeneratedMode(cfg)
	case "3":
		return promptImageMode(cfg)
	default:
		return promptDockerfileMode(cfg)
	}
}

// promptDockerfileMode configures provided-Dockerfile builds.
func promptDockerfileMode(cfg *wlconfig.Config) error {
	dockerfile, err := wlprompt.AskWithDefault("Dockerfile path", wlconfig.DefaultDockerfile)
	if err != nil {
		return err
	}

	cfg.Build.Dockerfile = dockerfile
	cfg.Build.Image = ""
	cfg.Build.ExecutionEnvironment = ""
	cfg.Build.Entrypoint = nil

	return nil
}

// promptGeneratedMode configures execution-environment builds.
func promptGeneratedMode(cfg *wlconfig.Config) error {
	ee, err := wlprompt.AskWithDefault("Execution environment (name)", wlconfig.DefaultExecutionEnvironment)
	if err != nil {
		return err
	}

	entry, err := wlprompt.AskWithDefault("Entrypoint", "uvicorn app:app --host 0.0.0.0 --port 8080")
	if err != nil {
		return err
	}

	cfg.Build.Dockerfile = ""
	cfg.Build.Image = ""
	cfg.Build.ExecutionEnvironment = ee
	cfg.Build.Entrypoint = strings.Fields(entry)

	return nil
}

// promptImageMode configures deployment of an existing container image
// (Tutorial 2 style): image URI, optional entrypoint override, and the health
// path since it is image-specific and gates the running transition.
func promptImageMode(cfg *wlconfig.Config) error {
	image, err := askFn("Image URI (e.g. registry/repo:tag)")
	if err != nil {
		return err
	}

	entry, err := wlprompt.AskWithDefault("Entrypoint (blank = image default)", "")
	if err != nil {
		return err
	}

	health, err := wlprompt.AskWithDefault("Health probe path", wlconfig.DefaultHealth)
	if err != nil {
		return err
	}

	cfg.Build.Image = image
	cfg.Build.Dockerfile = ""
	cfg.Build.ExecutionEnvironment = ""
	cfg.Build.Entrypoint = strings.Fields(entry)
	cfg.Build.Health = health

	return nil
}

// preserveRecordedWorkloadID carries an existing manifest's workloadId into the
// regenerated one. A re-run must not orphan the recorded workload: a draft
// artifact allows only one workload, so dropping the id leads straight to a 409
// on the next `up`. An explicitly chosen workload (non-empty cfg.WorkloadID)
// wins.
func preserveRecordedWorkloadID(cmd *cobra.Command, projectDir string, cfg *wlconfig.Config) {
	if cfg.WorkloadID != "" {
		return
	}

	prev, err := wlconfig.Load(projectDir)
	if err != nil || prev.WorkloadID == "" {
		return
	}

	cfg.WorkloadID = prev.WorkloadID

	fmt.Fprintf(cmd.ErrOrStderr(),
		"Keeping existing workload binding %s (remove the workloadId line from %s to detach).\n",
		prev.WorkloadID, wlconfig.Path(projectDir))
}

// selectWorkload resolves which workload the config binds to. Passing
// --workload-id or --name (with or without --yes) is a non-interactive request
// and wins; otherwise, on an interactive terminal, the picker runs. An empty
// returned id means "create a new workload named <name> on the first up".
func selectWorkload(yes bool, idFlag, nameFlag string) (id, name string, err error) {
	switch {
	case idFlag != "":
		return idFlag, nameFlag, nil
	case nameFlag != "":
		return "", nameFlag, nil
	}

	if yes || !isStdinTerminalFn() {
		return "", "", errors.New("non-interactive: provide --workload-id (existing) or --name (new)")
	}

	var workloads []workload.Workload

	if err := tui.RunWithSpinner("Fetching workloads", func() error {
		w, e := listWorkloadsFn(workloadPickLimit, nil)
		workloads = w

		return e
	}); err != nil {
		return "", "", err
	}

	// With no existing workloads there is nothing to pick, and an empty picker
	// is a dead end; go straight to naming a new one.
	if len(workloads) == 0 {
		return promptNewName()
	}

	item, err := runPickerFn(workloads)
	if err != nil {
		return "", "", err
	}

	if item.id == createNewID {
		return promptNewName()
	}

	return item.id, item.name, nil
}

// promptNewName asks for a workload name and returns it as the "create new"
// selection (empty id, so `up` creates it on first run).
func promptNewName() (id, name string, err error) {
	newName, err := askFn("Workload name")
	if err != nil {
		return "", "", err
	}

	return "", newName, nil
}

// runPicker runs the interactive workload picker and returns the chosen item.
func runPicker(workloads []workload.Workload) (workloadItem, error) {
	m := newPickerModel(workloads)

	finalModel, err := tui.Run(m, tea.WithAltScreen())
	if err != nil {
		return workloadItem{}, err
	}

	picker, ok := tui.Unwrap(finalModel).(pickerModel)
	if !ok {
		return workloadItem{}, errors.New("unexpected inner model type returned from TUI")
	}

	if picker.selected == nil {
		return workloadItem{}, errors.New("no workload selected")
	}

	return *picker.selected, nil
}

var checkStyle = lipgloss.NewStyle().Foreground(tui.GetAdaptiveColor(tui.DrGreen, tui.DrGreenDark))

func renderResult(cmd *cobra.Command, outputFormat outputformat.OutputFormat, cfg wlconfig.Config, path string) error {
	result := configResult{
		WorkloadID: cfg.WorkloadID,
		Name:       cfg.Name,
		Path:       path,
		CreateOnUp: cfg.WorkloadID == "",
	}

	if outputFormat == outputformat.OutputFormatJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")

		return enc.Encode(result)
	}

	w := cmd.ErrOrStderr()
	check := checkStyle.Render("✓")

	fmt.Fprintln(w)

	if cfg.WorkloadID != "" {
		fmt.Fprintf(w, "  %s Bound workload %s (%s)\n", check, cfg.WorkloadID, cfg.Name)
	} else {
		fmt.Fprintf(w, "  %s Recorded new workload %q (created on first `dr workload up`)\n", check, cfg.Name)
	}

	fmt.Fprintf(w, "  %s Wrote %s\n", check, path)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n", tui.HintStyle.Render("Run `dr workload up` to deploy."))
	fmt.Fprintln(cmd.OutOrStdout(), path)

	return nil
}
