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

package tools

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/misc/regexp2"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/internal/state"
	"github.com/datarobot/cli/internal/version"
)

// InstallCommands holds platform-specific install commands for a dependency.
type InstallCommands struct {
	MacOS   string `yaml:"macos"   validate:"required"`
	Linux   string `yaml:"linux"   validate:"required"`
	Windows string `yaml:"windows"`
}

// Prerequisite represents a required tool
type Prerequisite struct {
	Key            string
	Name           string          `yaml:"name"            validate:"required"`
	MinimumVersion string          `yaml:"minimum-version" validate:"required,semver"`
	Command        string          `yaml:"command"         validate:"required"`
	URL            string          `yaml:"url"             validate:"required"`
	Install        InstallCommands `yaml:"install"         validate:"required"`
}

// PlatformInstallCommand returns the install command for the current OS.
// Returns an error if no command is defined for this platform.
func (p Prerequisite) PlatformInstallCommand() (string, error) {
	var cmd string

	switch runtime.GOOS {
	case "darwin":
		cmd = p.Install.MacOS
	case "linux":
		cmd = p.Install.Linux
	case "windows":
		cmd = p.Install.Windows
	default:
		return "", fmt.Errorf("unsupported platform %q", runtime.GOOS)
	}

	if cmd == "" {
		return "", fmt.Errorf("no install command defined for %q on %s", p.Name, runtime.GOOS)
	}

	return cmd, nil
}

var pythonInstallCmd = InstallCommands{
	MacOS: "brew install python",
	Linux: "sudo apt-get install python3",
	// Windows: "Download and install Python from https://www.python.org/downloads/windows/",
}

var uvInstallCmd = InstallCommands{
	MacOS: "brew install uv",
	Linux: "curl -Ls https://astral.sh/uv/install.sh | sh",
	// Windows: "iwr -useb https://astral.sh/uv/install.ps1 | iex",
}

var taskInstallCmd = InstallCommands{
	MacOS: "brew install go-task/tap/go-task",
	Linux: "curl -sL https://taskfile.dev/install.sh | sh",
	// Windows: "iwr -useb https://taskfile.dev/install.ps1 | iex",
}

var pulumiInstallCmd = InstallCommands{
	MacOS: "brew install pulumi",
	Linux: "curl -fsSL https://get.pulumi.com | sh",
	// Windows: "iwr -useb https://get.pulumi.com/windows-installer.exe -OutFile pulumi-installer.exe; Start-Process -FilePath pulumi-installer.exe -Wait",
}

// RequiredTools lists all tools required for the quickstart process
var RequiredTools = []Prerequisite{
	{Name: "Python", Command: "python3 --version", URL: "https://www.python.org/downloads/", MinimumVersion: "3.9.6", Install: pythonInstallCmd},
	{Name: "uv", Command: "uv --version", URL: "https://docs.astral.sh/uv/getting-started/installation/", MinimumVersion: "0.11.20", Install: uvInstallCmd},
	{Name: "task", Command: "task --version", URL: "https://taskfile.dev/docs/installation", MinimumVersion: "3.50.0", Install: taskInstallCmd},
	{Name: "pulumi", Command: "pulumi version", URL: "https://www.pulumi.com/docs/get-started/download-install/", MinimumVersion: "3.245.0", Install: pulumiInstallCmd},
}

func CheckPrerequisite(name string) error {
	for _, tool := range RequiredTools {
		if tool.Name == name {
			if !isInstalled(tool.Command) {
				return fmt.Errorf("%s is not installed.", name)
			}
		}
	}

	return nil
}

// CheckResult holds the outcome of a CheckPrerequisites call.
type CheckResult struct {
	MissingTools         []Prerequisite
	WrongVersionTools    []Prerequisite
	MissingMsgs          []string
	WrongVersionMsgs     []string
	ValidationViolations []string
}

// CheckPrerequisites returns lists of missing prerequisites, wrongVersion prerequisites, and error messages to display to the user.
func CheckPrerequisites() CheckResult {
	prerequisites, violations, err := GetRequirements()
	if err == nil {
		RequiredTools = prerequisites
	}

	log.Debug("deps: checking prerequisites", "count", len(RequiredTools))

	result := CheckPrerequisiteList(RequiredTools)

	result.ValidationViolations = violations

	if len(result.MissingMsgs) == 0 && len(result.WrongVersionMsgs) == 0 {
		log.Debug("deps: all prerequisites satisfied")

		if repoRoot, err := repo.FindRepoRoot(); err == nil {
			err := state.UpdateAfterSuccessDepsCheck(repoRoot)
			if err != nil {
				log.Errorf("Failed to update state AfterSuccessDepsCheck: %v", err)
			}
		}
	}

	return result
}

// CheckPrerequisiteList checks an arbitrary list of prerequisites and returns the result.
func CheckPrerequisiteList(prereqs []Prerequisite) CheckResult {
	var result CheckResult

	for _, tool := range prereqs {
		if !isInstalled(tool.Command) {
			log.Debug("deps: tool missing", "name", tool.Name)

			result.MissingTools = append(result.MissingTools, tool)
			result.MissingMsgs = append(result.MissingMsgs, fmt.Sprintf("%s %s (%s)", tool.Name, tool.MinimumVersion, tool.URL))
		} else if ver, ok := isVersionInstalled(tool); !ok {
			log.Debug("deps: tool wrong version", "name", tool.Name, "msg", ver)

			result.WrongVersionTools = append(result.WrongVersionTools, tool)
			result.WrongVersionMsgs = append(result.WrongVersionMsgs, ver)
		} else {
			log.Debug("deps: tool ok", "name", tool.Name)
		}
	}

	return result
}

// PrerequisitesMsg formats the message to display to the user about missing/wrong-version prerequisites.
func PrerequisitesMsg(missingMsgs []string, wrongVersionMsgs []string) string {
	result := make([]string, 0)

	if len(missingMsgs) > 0 {
		result = append(result, "\n ❌ Missing required tools:\n")

		for _, msg := range missingMsgs {
			result = append(result, "\t- "+msg)
		}
	}

	if len(wrongVersionMsgs) > 0 {
		result = append(result, "\n ⚠️ Wrong versions of tools:\n")

		for _, msg := range wrongVersionMsgs {
			result = append(result, "\t- "+msg)
		}
	}

	return strings.Join(result, "\n") + "\n"
}

func commandArgs(fullCommand string) (string, []string) {
	command := strings.Split(fullCommand, " ")

	if len(command) == 0 {
		return "", nil
	}

	return command[0], command[1:]
}

// isInstalled checks if a command is available in the system PATH
func isInstalled(fullCommand string) bool {
	command, _ := commandArgs(fullCommand)

	if command == "dr" {
		return true
	}

	_, err := exec.LookPath(command)

	return err == nil
}

// isVersionInstalled checks if a command has proper version installed
func isVersionInstalled(tool Prerequisite) (string, bool) {
	// Return success result if no version or no version command specified
	if tool.MinimumVersion == "" || tool.Command == "" {
		return "", true
	}

	if tool.Key == "dr" {
		if !SufficientSelfVersion(tool.MinimumVersion) {
			return fmt.Sprintf("%s (minimal: v%s, installed: %s)\n%s\n",
				tool.Name, tool.MinimumVersion, version.Version, tool.URL), false
		}

		return "", true
	}

	command, args := commandArgs(tool.Command)

	versionOutput, err := exec.Command(command, args...).Output()
	if err != nil {
		return fmt.Sprintf("%s (minimal: v%s, installed: unknown)\n%s\n",
			tool.Name, tool.MinimumVersion, tool.URL), false
	}

	if versionInstalled, ok := sufficientVersion(string(versionOutput), tool.MinimumVersion); !ok {
		return fmt.Sprintf("%s (minimal: v%s, installed: %s)\n%s\n",
			tool.Name, tool.MinimumVersion, versionInstalled, tool.URL), false
	}

	return "", true
}

func SufficientSelfVersion(minimal string) bool {
	if version.Version == "dev" {
		return true
	}

	if minimal == "" {
		return false
	}

	_, sufficient := sufficientVersion(version.Version, minimal)

	return sufficient
}

func sufficientVersion(versionOutput, minimalStr string) (string, bool) {
	expr := regexp.MustCompile(`v?(?P<major>\d+)(.(?P<minor>\d+)(.(?P<patch>\d+))?)?`)
	installed := regexp2.NamedIntMatches(expr, versionOutput)
	minimal := regexp2.NamedIntMatches(expr, minimalStr)

	installedStr := fmt.Sprintf("v%d.%d.%d", installed["major"], installed["minor"], installed["patch"])

	if installed["major"] < minimal["major"] {
		return installedStr, false
	} else if installed["major"] == minimal["major"] && installed["minor"] < minimal["minor"] {
		return installedStr, false
	} else if installed["major"] == minimal["major"] && installed["minor"] == minimal["minor"] && installed["patch"] < minimal["patch"] {
		return installedStr, false
	}

	return installedStr, true
}

// CheckTool verifies if a specific tool is installed
func CheckTool(name string) error {
	for _, tool := range RequiredTools {
		if tool.Name == name {
			if !isInstalled(tool.Command) {
				return fmt.Errorf("%s is not installed.", name)
			}

			return nil
		}
	}

	return fmt.Errorf("Unknown tool: %s.", name)
}

// GetMissingTools returns a list of missing prerequisite tools
func GetMissingTools() []string {
	var missing []string

	for _, tool := range RequiredTools {
		if !isInstalled(tool.Command) {
			missing = append(missing, tool.Name)
		}
	}

	return missing
}
