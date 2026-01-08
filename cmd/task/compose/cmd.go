// Copyright 2025 DataRobot, Inc. and its affiliates.
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

package compose

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/task"
	"github.com/spf13/cobra"
)

var templatePath string

func Run(_ *cobra.Command, _ []string) {
	taskfileName := detectExistingTaskfile()
	discovery := createDiscovery(taskfileName)

	taskFilePath, err := discovery.Discover(".", 2)
	if err != nil {
		task.ExitWithError(err)
		return
	}

	fmt.Printf("Generated file saved to: %s\n", taskFilePath)

	contentBytes, err := os.ReadFile(".gitignore")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Error(fmt.Errorf("Failed to read from '.gitignore' file: %w", err))
		return
	}

	contents := string(contentBytes)
	taskfileIgnore := "/" + taskfileName

	// Check if Taskfile.yaml or Taskfile.yml is already in .gitignore
	if isIgnored(contents, taskfileIgnore) {
		return
	}

	f, err := os.Create(".gitignore")
	if err != nil {
		log.Error(fmt.Errorf("Failed to create '.gitignore' file: %w", err))
		return
	}

	defer f.Close()

	_, err = f.WriteString(taskfileIgnore + "\n\n" + contents)
	if err != nil {
		log.Error(fmt.Errorf("Failed to write to '.gitignore' file: %w", err))
		return
	}

	fmt.Println("Added " + taskfileIgnore + " line to '.gitignore'.")
}

func createDiscovery(taskfileName string) *task.Discovery {
	// Check for .Taskfile.template in the root directory if no template specified
	autoTemplatePath := ".Taskfile.template"

	if templatePath == "" {
		if _, err := os.Stat(autoTemplatePath); err == nil {
			templatePath = autoTemplatePath
			fmt.Printf("Using auto-discovered template: %s\n", autoTemplatePath)
		}
	}

	// If template is specified or found, use compose mode
	if templatePath != "" {
		absPath, err := validateTemplatePath(templatePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		return task.NewComposeDiscovery(taskfileName, absPath)
	}

	return task.NewTaskDiscovery(taskfileName)
}

func validateTemplatePath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving template path: %w", err)
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return "", fmt.Errorf("template file not found: %s", absPath)
	}

	return absPath, nil
}

// detectExistingTaskfile checks for existing Taskfile.yaml or Taskfile.yml
// and returns the name of the existing one, or defaults to Taskfile.yaml
func detectExistingTaskfile() string {
	// Check for Taskfile.yaml first (more common)
	if _, err := os.Stat("Taskfile.yaml"); err == nil {
		return "Taskfile.yaml"
	}

	// Check for Taskfile.yml
	if _, err := os.Stat("Taskfile.yml"); err == nil {
		return "Taskfile.yml"
	}

	// Default to Taskfile.yaml if neither exists
	return "Taskfile.yaml"
}

// isIgnored checks if a pattern is already in .gitignore content
func isIgnored(content, pattern string) bool {
	// Normalize content to have trailing newline for consistent checking
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	// Check if pattern exists as a complete line
	return strings.Contains(content, "\n"+pattern+"\n") || strings.HasPrefix(content, pattern+"\n")
}

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compose",
		Short: "Compose 'Taskfile.yaml' from multiple files in subdirectories",
		Long: `Compose a root Taskfile.yaml by discovering Taskfiles in subdirectories.

By default, generates a simple Taskfile with includes only.

If a .Taskfile.template file is found in the root directory, it will be used
automatically to generate a more comprehensive Taskfile with aggregated tasks.

You can also specify a custom template with the --template flag.`,
		Run: Run,
	}

	cmd.Flags().StringVarP(&templatePath, "template", "t", "", "Path to custom Taskfile template")

	return cmd
}
