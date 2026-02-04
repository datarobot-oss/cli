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

package copier

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/charmbracelet/log"
)

func cmdRun(cmd *exec.Cmd) error {
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	return cmd.Run()
}

type AddFlags struct {
	DataArgs []string
	DataFile string
}

// Add creates a copier copy command with optional --data arguments
func Add(repoURL string, data map[string]interface{}) *exec.Cmd {
	commandParts := []string{"copier", "copy", repoURL, "."}

	for key, value := range data {
		commandParts = append(commandParts, "--data", key+"="+formatDataValue(value))
	}

	cmd := exec.Command("uvx", commandParts...)
	log.Debug("Running command: " + cmd.String())

	// Suppress all Python warnings unless debug mode is enabled
	if log.GetLevel() >= log.WarnLevel {
		cmd.Env = append(os.Environ(), "PYTHONWARNINGS=ignore")
	}

	return cmd
}

// ExecAdd executes a copier copy command with optional --data arguments
func ExecAdd(repoURL string, data map[string]interface{}) error {
	if repoURL == "" {
		return errors.New("Repository URL is missing.")
	}

	return cmdRun(Add(repoURL, data))
}

type UpdateFlags struct {
	DataArgs  []string
	DataFile  string
	Recopy    bool
	VcsRef    string
	Quiet     bool
	Overwrite bool
}

// Update creates a copier update command with optional --data arguments
func Update(yamlFile string, data map[string]interface{}, flags UpdateFlags) *exec.Cmd {
	copierCommand := "update"

	if flags.Recopy {
		copierCommand = "recopy"
	}

	commandParts := []string{
		"copier", copierCommand, "--answers-file", yamlFile, "--skip-answered",
	}

	if flags.VcsRef != "" {
		commandParts = append(commandParts, "--vcs-ref", flags.VcsRef)
	}

	if flags.Quiet {
		commandParts = append(commandParts, "--quiet")
	}

	if flags.Recopy && flags.Overwrite {
		commandParts = append(commandParts, "--overwrite")
	}

	for key, value := range data {
		commandParts = append(commandParts, "--data", key+"="+formatDataValue(value))
	}

	cmd := exec.Command("uvx", commandParts...)
	log.Debug("Running command: " + cmd.String())

	// Suppress all Python warnings unless debug mode is enabled
	if log.GetLevel() >= log.WarnLevel {
		cmd.Env = append(os.Environ(), "PYTHONWARNINGS=ignore")
	}

	return cmd
}

// ExecUpdate executes a copier update command with optional --data arguments
func ExecUpdate(yamlFile string, data map[string]interface{}, flags UpdateFlags) error {
	if yamlFile == "" {
		return errors.New("Path to YAML file is missing.")
	}

	return cmdRun(Update(yamlFile, data, flags))
}

// formatDataValue converts a value to a string suitable for --data arguments
// This follows copier's type handling: str, int, float, bool, json, yaml
func formatDataValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case bool:
		return formatBool(v)
	case []interface{}:
		// Handle arrays/slices - format as YAML list for multiselect choices
		return formatYAMLList(v)
	case map[string]interface{}:
		// Handle objects - format as YAML/JSON
		return formatYAMLMap(v)
	case nil:
		return "null"
	default:
		// Handle all numeric types
		return formatNumeric(v)
	}
}

// formatBool formats a boolean value
func formatBool(v bool) string {
	if v {
		return "true"
	}

	return "false"
}

// formatNumeric formats numeric types using strconv for performance
func formatNumeric(value interface{}) string {
	switch v := value.(type) {
	case int:
		return strconv.Itoa(v)
	case int8:
		return strconv.FormatInt(int64(v), 10)
	case int16:
		return strconv.FormatInt(int64(v), 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case float32:
		return strconv.FormatFloat(float64(v), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	default:
		// Fallback to string representation
		return fmt.Sprintf("%v", v)
	}
}

// formatYAMLList formats a slice as a YAML-style list string
// e.g., [1, 2, 3] for multiselect choice questions
func formatYAMLList(items []interface{}) string {
	strItems := make([]string, len(items))
	for i, item := range items {
		strItems[i] = formatDataValue(item)
	}

	return "[" + strings.Join(strItems, ", ") + "]"
}

// formatYAMLMap formats a map as a YAML string for complex data types
func formatYAMLMap(data map[string]interface{}) string {
	parts := make([]string, 0, len(data))
	for k, v := range data {
		parts = append(parts, fmt.Sprintf("%s: %s", k, formatDataValue(v)))
	}

	return "{" + strings.Join(parts, ", ") + "}"
}
