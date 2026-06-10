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

package add

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/cmd/component/shared"
	"github.com/datarobot/cli/cmd/dotenv"
	"github.com/datarobot/cli/cmd/helpers"
	"github.com/datarobot/cli/cmd/task/compose"
	"github.com/datarobot/cli/internal/appframework"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/tools"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

type addFlags struct {
	DataArgs []string
	DataFile string
	Label    string
}

var flags addFlags

func PreRunE(_ *cobra.Command, _ []string) error {
	if !repo.IsInRepoRoot() {
		if repo.IsInRepo() {
			return errors.New("You must be in the repository root directory.")
		}

		if err := bootstrapCliDir(); err != nil {
			return fmt.Errorf("initializing project directory: %w", err)
		}
	}

	if err := tools.CheckPrerequisite("uv"); err != nil {
		return err
	}

	return nil
}

func bootstrapCliDir() error {
	cliDir := filepath.Join(".datarobot", "cli")

	if err := os.MkdirAll(cliDir, 0o755); err != nil {
		return fmt.Errorf("creating .datarobot/cli: %w", err)
	}

	keepPath := filepath.Join(cliDir, ".keep")

	if _, err := os.Stat(keepPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("checking .keep: %w", err)
		}

		if err := os.WriteFile(keepPath, []byte{}, 0o644); err != nil {
			return fmt.Errorf("creating .keep: %w", err)
		}
	}

	gitignorePath := filepath.Join(cliDir, ".gitignore")

	existing, _ := os.ReadFile(gitignorePath)

	if strings.Contains(string(existing), ".keep") {
		return nil
	}

	f, err := os.OpenFile(gitignorePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening .gitignore: %w", err)
	}

	defer f.Close()

	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		if _, err := fmt.Fprintln(f); err != nil {
			return fmt.Errorf("writing .gitignore: %w", err)
		}
	}

	if _, err := fmt.Fprintln(f, ".keep"); err != nil {
		return fmt.Errorf("writing .gitignore: %w", err)
	}

	return nil
}

func RunE(cmd *cobra.Command, args []string) error {
	fw := shared.GetFrameworkPath()

	// Step 1: Initialize the framework (idempotent).
	if err := appframework.ExecInitializeFramework(fw); err != nil {
		return fmt.Errorf("initializing framework: %w", err)
	}

	// Step 2: Ensure the default registry is registered.
	if err := ensureDefaultRegistry(fw); err != nil {
		return fmt.Errorf("ensuring default registry: %w", err)
	}

	// Step 3: Pick module names (from CLI args or TUI picker).
	moduleNames, err := getArgsFromCLIOrPrompt(args, fw)
	if err != nil {
		return err
	}

	if len(moduleNames) == 0 || moduleNames[0] == "" {
		return errors.New("A component module name is required.")
	}

	// Steps 4-8: For each module, add it and copy it.
	yesFlag, _ := cmd.Flags().GetBool("yes")
	yes := yesFlag || viperx.GetBool("yes")

	if err := addModules(moduleNames, fw, yes); err != nil {
		return err
	}

	// Step 9: Regenerate the root Taskfile.yaml.
	if err := compose.Cmd().RunE(nil, nil); err != nil {
		return err
	}

	// Step 10: Validate and edit .env if needed.
	if err := dotenv.ValidateAndEditIfNeeded(); err != nil {
		// Log warning but don't fail - the component was successfully added.
		log.Warn("Environment configuration may need manual updates")
	}

	return nil
}

// ensureDefaultRegistry adds the default "core" registry if it isn't already registered.
func ensureDefaultRegistry(fw string) error {
	aliases, err := appframework.RegistryAliases(fw, ".")
	if err != nil {
		return err
	}

	if aliases["core"] {
		return nil
	}

	fmt.Println("Adding default registry...")

	return appframework.ExecAddRegistry(appframework.RegistryURI(), "core", fw)
}

// getArgsFromCLIOrPrompt returns module names from CLI args or from the TUI picker.
func getArgsFromCLIOrPrompt(args []string, fw string) ([]string, error) {
	if len(args) > 0 {
		return args, nil
	}

	am := shared.NewAddModel(fw)

	finalModel, err := tui.Run(am, tea.WithAltScreen())
	if err != nil {
		return nil, err
	}

	if startModel, ok := finalModel.(tui.InterruptibleModel); ok {
		if innerModel, ok := startModel.Model.(shared.AddModel); ok {
			return innerModel.ModuleNames, nil
		}
	}

	return args, nil
}

// addModules runs the multi-step add flow (add-module → answer → copy → run-tasks) for each module.
func addModules(moduleNames []string, fw string, nonInteractive bool) error {
	cliData, err := shared.ParseDataArgs(flags.DataArgs)
	if err != nil {
		return fmt.Errorf("parsing data args: %w", err)
	}

	componentConfig := loadComponentDefaults(flags.DataFile)

	installedModules, err := appframework.ListInstalled(fw, ".")
	if err != nil {
		return fmt.Errorf("checking installed modules: %w", err)
	}

	installedNames := make(map[string]bool, len(installedModules))

	for _, inst := range installedModules {
		installedNames[inst.Module] = true
	}

	var addedLabels []string

	for _, moduleName := range moduleNames {
		if installedNames[moduleName] && !nonInteractive {
			fmt.Printf("Module %q is already installed in this application.\n", moduleName)

			confirmed, err := helpers.Confirm(os.Stdout, os.Stdin, "Add it again? [y/N] ")
			if err != nil {
				return fmt.Errorf("reading confirmation: %w", err)
			}

			if !confirmed {
				fmt.Printf("Skipping module %s.\n", moduleName)

				continue
			}
		}

		fmt.Printf("Adding module: %s\n", moduleName)

		// Step 4: Add the module to instance state, get its assigned label.
		label, err := appframework.ExecAddModule(moduleName, flags.Label, fw, ".", nil)
		if err != nil {
			// TODO: Check beforehand if uv is installed or not.
			if errors.Is(err, exec.ErrNotFound) {
				log.Error("uv is not installed.")
			}

			return fmt.Errorf("adding module %q: %w", moduleName, err)
		}

		fmt.Printf("Module %s added as %s\n", moduleName, label)

		// Step 5: Merge --data-file + --data args.
		mergedData := componentConfig.MergeWithCLIData(moduleName, cliData)

		// Step 6: Pre-supply known answers.
		if err := appframework.ExecAnswer(label, mergedData, fw, "."); err != nil {
			return fmt.Errorf("answering questions for %q: %w", label, err)
		}

		addedLabels = append(addedLabels, label)
	}

	if len(addedLabels) == 0 {
		return nil
	}

	// Step 7: Copy templates for all added modules in one pass. Running copy
	// per-module would fail on the second module because the framework requires a
	// clean git state for updates, but the first copy leaves uncommitted files.
	if err := appframework.ExecCopy(fw, ".", nonInteractive); err != nil {
		return fmt.Errorf("copying templates: %w", err)
	}

	// Step 8: Run post-copy tasks from .phantom/.
	if err := appframework.ExecRunTasks("."); err != nil {
		return fmt.Errorf("running tasks: %w", err)
	}

	for _, label := range addedLabels {
		fmt.Printf("Module %s installed successfully.\n", label)
	}

	return nil
}

func loadComponentDefaults(dataFilePath string) *config.ComponentDefaults {
	componentConfig, err := config.LoadComponentDefaults(dataFilePath)
	if err != nil {
		log.Warn("Failed to load component defaults", "error", err)

		componentConfig = &config.ComponentDefaults{
			Defaults: make(map[string]map[string]interface{}),
		}
	}

	return componentConfig
}

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add [module_name]",
		Short:   "➕ Add a component",
		PreRunE: PreRunE,
		RunE:    RunE,
	}

	cmd.Flags().StringArrayVarP(&flags.DataArgs, "data", "d", []string{}, "Provide answer data in key=value format (can be specified multiple times)")
	cmd.Flags().StringVar(&flags.DataFile, "data-file", "", "Path to YAML file with default answers")
	cmd.Flags().StringVar(&flags.Label, "label", "", "Explicit label for the module instance (e.g. core.agent.2)")
	cmd.Flags().BoolP("yes", "y", false, "Skip interactive prompts; fail if required answers are missing.")

	// Bind only the env var so --yes does not leak into drconfig.yaml via viper.AllSettings().
	_ = viperx.BindEnv("yes", "DATAROBOT_CLI_NON_INTERACTIVE")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"component_name": telemetry.FirstArg(args),
		}
	})

	return cmd
}
