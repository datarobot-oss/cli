// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package copier

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func Add(repoURL string) *exec.Cmd {
	return exec.Command("uvx", "copier", "copy", repoURL, ".")
}

func ExecAdd(repoURL string) error {
	if repoURL == "" {
		return errors.New("repository URL is missing")
	}

	cmd := Add(repoURL)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	return cmd.Run()
}

// AddWithData creates a copier copy command with --data arguments
func AddWithData(repoURL string, data map[string]interface{}) *exec.Cmd {
	args := []string{"copier", "copy", repoURL, "."}

	for key, value := range data {
		args = append(args, "--data", key+"="+formatDataValue(value))
	}

	return exec.Command("uvx", args...)
}

// ExecAddWithData executes a copier copy command with --data arguments
func ExecAddWithData(repoURL string, data map[string]interface{}) error {
	if repoURL == "" {
		return errors.New("repository URL is missing")
	}

	cmd := AddWithData(repoURL, data)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	return cmd.Run()
}

func Update(yamlFile string, quiet bool) *exec.Cmd {
	commandParts := []string{
		"copier", "update", "--answers-file", yamlFile, "--skip-answered",
	}
	if quiet {
		commandParts = append(commandParts, "--quiet")
	}

	return exec.Command("uvx", commandParts...)
}

func ExecUpdate(yamlFile string, quiet bool) error {
	if yamlFile == "" {
		return errors.New("path to yaml file is missing")
	}

	cmd := Update(yamlFile, quiet)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	return cmd.Run()
}

// UpdateWithData creates a copier update command with --data arguments
func UpdateWithData(yamlFile string, data map[string]interface{}) *exec.Cmd {
	args := []string{"copier", "update", "-a", yamlFile}

	for key, value := range data {
		args = append(args, "--data", key+"="+formatDataValue(value))
	}

	return exec.Command("uvx", args...)
}

// ExecUpdateWithData executes a copier update command with --data arguments
func ExecUpdateWithData(yamlFile string, data map[string]interface{}) error {
	if yamlFile == "" {
		return errors.New("path to yaml file is missing")
	}

	cmd := UpdateWithData(yamlFile, data)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	return cmd.Run()
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
