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

package appframework

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/datarobot/cli/internal/log"
)

// AFSourcePath is the default --from target for uvx. Points to the local dr-application-framework
// CLI checkout for this experimental branch. Override with DR_APP_FRAMEWORK_PATH for local dev.
// Once app-framework-cli is published to PyPI, change this to just "app-framework-cli".
const AFSourcePath = "/Users/damon.stanley/workspace/dr-application-framework/cli"

const defaultRegistryURI = "https://raw.githubusercontent.com/datarobot/dr-application-framework/main/registry.yml"

// afSourcePath returns the uvx --from target. DR_APP_FRAMEWORK_PATH overrides the hardcoded
// default, useful for pointing at a local checkout during development.
func afSourcePath() string {
	if p := os.Getenv("DR_APP_FRAMEWORK_PATH"); p != "" {
		return p
	}

	return AFSourcePath
}

// RegistryURI returns the registry manifest URL. The env var DR_APP_FRAMEWORK_REGISTRY_URI
// overrides the default, useful for pointing at a local file:// path during development.
func RegistryURI() string {
	if uri := os.Getenv("DR_APP_FRAMEWORK_REGISTRY_URI"); uri != "" {
		return uri
	}

	return defaultRegistryURI
}

// afCommand builds a dr-app-framework subprocess invocation.
// Usage: afCommand("add-module", "-m", "core.agent", ...)
func afCommand(subcmd string, args ...string) *exec.Cmd {
	src := afSourcePath()

	var cmd *exec.Cmd

	if filepath.IsAbs(src) {
		// Local checkout: use `uv run --project <path>` so the tool runs directly
		// from source, bypassing uvx's package cache which won't pick up code changes
		// without a version bump.
		all := make([]string, 0, 4+len(args))
		all = append(all, "run", "--project", src, "dr-app-framework", subcmd)
		all = append(all, args...)
		cmd = exec.Command("uv", all...)
	} else {
		all := make([]string, 0, 4+len(args))
		all = append(all, "--from", src, "dr-app-framework", subcmd)
		all = append(all, args...)
		cmd = exec.Command("uvx", all...)
	}

	log.Debug("Running command: " + cmd.String())

	// Suppress all Python warnings unless debug mode is enabled.
	if log.GetLevel() >= log.WarnLevel {
		cmd.Env = append(os.Environ(), "PYTHONWARNINGS=ignore")
	}

	return cmd
}

// cmdRun executes a command inheriting the parent's stdio (for interactive commands such
// as copy and run-tasks that may prompt the user).
func cmdRun(cmd *exec.Cmd) error {
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	return cmd.Run()
}

// cmdOutput executes a command, capturing stdout as bytes. stderr passes through so the
// user can see any warnings or progress output. Used for JSON-returning subcommands.
func cmdOutput(cmd *exec.Cmd) ([]byte, error) {
	var out bytes.Buffer

	cmd.Stdout = &out
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

// ExecInitializeFramework creates the framework directory structure (idempotent).
func ExecInitializeFramework(fw string) error {
	return cmdRun(afCommand("initialize-framework", "-f", fw, "-t", "."))
}

// ExecAddRegistry registers a registry URI with a local alias.
func ExecAddRegistry(uri, alias, fw string) error {
	return cmdRun(afCommand("add-registry", uri, "--alias", alias, "-f", fw))
}

// AddModuleResponse mirrors the JSON returned by dr-app-framework add-module.
type AddModuleResponse struct {
	AddedModules map[string]string `json:"added_modules"`
}

// ExecAddModule adds a module to the instance state and returns the label assigned to it.
// label may be empty to let the framework auto-assign (e.g. "core.agent.1").
// deps maps DisambiguatedModuleName -> Label for explicit dependency wiring.
func ExecAddModule(module, label, fw, target string, deps map[string]string) (string, error) {
	args := []string{"-m", module, "-f", fw, "-t", target}

	if label != "" {
		args = append(args, "-l", label)
	}

	for k, v := range deps {
		args = append(args, "-d", k+"="+v)
	}

	data, err := cmdOutput(afCommand("add-module", args...))
	if err != nil {
		return "", fmt.Errorf("add-module %s: %w", module, err)
	}

	var resp AddModuleResponse

	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parsing add-module response: %w", err)
	}

	// Return the label for the requested module. When the module name is the disambiguated
	// key (e.g. "core.agent"), do a direct lookup first.
	if lbl, ok := resp.AddedModules[module]; ok {
		return lbl, nil
	}

	// Fallback: find any entry whose key has module as a suffix (e.g. key "core.agent" for
	// short name "agent"). Return the first match.
	for _, lbl := range resp.AddedModules {
		return lbl, nil
	}

	return "", errors.New("add-module returned no labels")
}

// ExecAnswer pre-supplies question answers for a label before the copy step.
// answers keys are question names or disambiguated names; values are formatted as YAML-safe strings.
func ExecAnswer(label string, answers map[string]interface{}, fw, target string) error {
	if len(answers) == 0 {
		return nil
	}

	// Sort keys for deterministic command construction and easier debugging.
	keys := make([]string, 0, len(answers))

	for k := range answers {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	args := []string{"-l", label, "-f", fw, "-t", target}

	for _, k := range keys {
		args = append(args, "-a", k+"="+formatDataValue(answers[k]))
	}

	return cmdRun(afCommand("answer", args...))
}

// ExecCopy runs the copy step, which populates the target directory from templates.
// Interactive: stdio is passed through so the user can answer any remaining questions.
func ExecCopy(fw, target string) error {
	return cmdRun(afCommand("copy", "-f", fw, "-t", target))
}

// UpdateCmd returns the update *exec.Cmd without running it.
// Used by the TUI (tea.ExecProcess) so Bubble Tea can manage the subprocess lifecycle.
func UpdateCmd(filter []string, fw, target string) *exec.Cmd {
	args := make([]string, 0, 4+2*len(filter))
	args = append(args, "-f", fw, "-t", target)

	for _, f := range filter {
		args = append(args, "-F", f)
	}

	cmd := afCommand("update", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd
}

// ExecUpdate runs the three-way merge update.
// filter optionally restricts the update to specific labels.
func ExecUpdate(filter []string, fw, target string) error {
	args := make([]string, 0, 4+2*len(filter))
	args = append(args, "-f", fw, "-t", target)

	for _, f := range filter {
		args = append(args, "-F", f)
	}

	return cmdRun(afCommand("update", args...))
}

// ExecRunTasks executes post-copy/update tasks in the .phantom/ directory, then removes it.
func ExecRunTasks(target string) error {
	return cmdRun(afCommand("run-tasks", target))
}

// --- Value formatters (mirrors internal/copier/exec.go for --answer/-a key=value args) ---

// formatDataValue converts a value to a string suitable for -a / -d arguments.
func formatDataValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case bool:
		return formatBool(v)
	case []interface{}:
		return formatYAMLList(v)
	case map[string]interface{}:
		return formatYAMLMap(v)
	case nil:
		return "null"
	default:
		return formatNumeric(v)
	}
}

func formatBool(v bool) string {
	if v {
		return "true"
	}

	return "false"
}

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
		return fmt.Sprintf("%v", v)
	}
}

func formatYAMLList(items []interface{}) string {
	strItems := make([]string, len(items))

	for i, item := range items {
		strItems[i] = formatDataValue(item)
	}

	return "[" + strings.Join(strItems, ", ") + "]"
}

func formatYAMLMap(data map[string]interface{}) string {
	parts := make([]string, 0, len(data))

	for k, v := range data {
		parts = append(parts, fmt.Sprintf("%s: %s", k, formatDataValue(v)))
	}

	return "{" + strings.Join(parts, ", ") + "}"
}
