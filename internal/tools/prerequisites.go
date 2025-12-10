// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package tools

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/datarobot/cli/internal/misc/regexp2"
	"github.com/datarobot/cli/internal/version"
)

// Prerequisite represents a required tool
type Prerequisite struct {
	Key            string
	Name           string `yaml:"name"`
	MinimumVersion string `yaml:"minimum-version"`
	Command        string `yaml:"command"`
	URL            string `yaml:"url"`
}

// RequiredTools lists all tools required for the quickstart process
var RequiredTools = []Prerequisite{
	{Name: "Python", Command: "python3", URL: "https://www.python.org/downloads/"},
	{Name: "uv", Command: "uv", URL: "https://docs.astral.sh/uv/getting-started/installation/"},
	{Name: "task", Command: "task", URL: "https://taskfile.dev/docs/installation"},
	{Name: "pulumi", Command: "pulumi", URL: "https://www.pulumi.com/docs/get-started/download-install/"},
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

// MissingPrerequisites verifies that all required tools are installed
func MissingPrerequisites() string {
	prerequisites, err := GetRequirements()
	if err == nil {
		RequiredTools = prerequisites
	}

	var (
		missing      []string
		wrongVersion []string
	)

	for _, tool := range RequiredTools {
		if !isInstalled(tool.Command) {
			missing = append(missing, tool.Name)
		} else if ver, ok := isVersionInstalled(tool); !ok {
			wrongVersion = append(wrongVersion, ver)
		}
	}

	if len(missing) == 0 && len(wrongVersion) == 0 {
		return ""
	}

	result := make([]string, 0)

	if len(missing) > 0 {
		result = append(result, fmt.Sprintf("Missing required tools:\n\n%s", strings.Join(missing, "\n")))
	}

	if len(wrongVersion) > 0 {
		result = append(result, fmt.Sprintf("Wrong versions of tools:\n\n%s", strings.Join(wrongVersion, "\n")))
	}

	return strings.Join(result, "\n")
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
