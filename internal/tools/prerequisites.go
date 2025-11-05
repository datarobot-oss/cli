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
	"strings"
)

// Prerequisite represents a required tool
type Prerequisite struct {
	Name    string
	Command string
	installString string
}

// RequiredTools lists all tools required for the quickstart process
var RequiredTools = []Prerequisite{
	{Name: "Python", Command: "python3", installString: "pip install python3"},
	{Name: "uv", Command: "uv", installString: "pip install uv"},
	{Name: "task", Command: "task", installString: "brew install task"},
	{Name: "pulumi", Command: "pulumi", installString: "brew install pulumi"},
}

func CheckPrerequisite(name string) error {
	for _, tool := range RequiredTools {
		if tool.Name == name {
			if !isInstalled(tool.Command) {
				return fmt.Errorf("%s is not installed", name)
			}
		}
	}

	return nil
}

// CheckPrerequisites verifies that all required tools are installed
func CheckPrerequisites() error {
	var missing []string

	for _, tool := range RequiredTools {
		if !isInstalled(tool.Command) {
			missing = append(missing, tool.Name)
		}
	}

	if len(missing) > 0 {
		fmt.Println("Please install the following tools:")
		for _, tool := range RequiredTools {
			if !isInstalled(tool.Command) {
				fmt.Printf(" - %s: %s\n", tool.Name, tool.installString)
			}
		}
		return fmt.Errorf("missing required tools: %s", strings.Join(missing, ", "))
	}

	return nil
}

// isInstalled checks if a command is available in the system PATH
func isInstalled(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

// CheckTool verifies if a specific tool is installed
func CheckTool(name string) error {
	for _, tool := range RequiredTools {
		if tool.Name == name {
			if !isInstalled(tool.Command) {
				return fmt.Errorf("%s is not installed", name)
			}

			return nil
		}
	}

	return fmt.Errorf("unknown tool: %s", name)
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
