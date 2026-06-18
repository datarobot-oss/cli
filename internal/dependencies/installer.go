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

package dependencies

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"

	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/internal/state"
	"github.com/datarobot/cli/internal/tools"
)

// InstallPrerequisites installs each prerequisite sequentially. It returns the names
// of tools successfully installed before any failure, plus the first error encountered.
func InstallPrerequisites(w io.Writer, prerequisites []tools.Prerequisite) ([]string, error) {
	log.Debug("deps: installing prerequisites", "count", len(prerequisites))

	var installed []string

	for _, prerequisite := range prerequisites {
		installCmd, err := prerequisite.PlatformInstallCommand()
		if err != nil {
			log.Debug("deps: no install command", "name", prerequisite.Name, "err", err)

			return installed, err
		}

		log.Debug("deps: running install", "name", prerequisite.Name, "cmd", installCmd)

		fmt.Fprintf(w, "📦 Installing %s...\n", prerequisite.Name)

		var cmdBuf bytes.Buffer

		exitCode, err := ExecuteShLine(installCmd, io.MultiWriter(w, &cmdBuf))
		if err != nil {
			log.Debug("deps: install failed to start", "name", prerequisite.Name, "err", err)

			return installed, fmt.Errorf("failed to start install for %q: %w\n  command: %s", prerequisite.Name, err, installCmd)
		}

		if exitCode != 0 {
			log.Debug("deps: install exited non-zero", "name", prerequisite.Name, "exit_code", exitCode)

			env := DetectEnvironment()
			permDenied := isPermissionDenied(exitCode, cmdBuf.String())
			msg := buildInstallFailureMsg(prerequisite, exitCode, permDenied, env, runtime.GOOS)

			fmt.Fprint(w, msg)

			return installed, fmt.Errorf("install failed for %q (exit code %d)", prerequisite.Name, exitCode)
		}

		log.Debug("deps: tool installed", "name", prerequisite.Name)

		fmt.Fprintf(w, "✅ %s installed\n", prerequisite.Name)

		installed = append(installed, prerequisite.Name)
	}

	log.Debug("deps: all prerequisites installed", "count", len(installed))

	fmt.Fprintf(w, "\n✅ All dependencies installed successfully.\n")

	// Update state after successful installs.
	// Executed only after all installs succeed to avoid state inconsistency if an install fails.
	if repoRoot, err := repo.FindRepoRoot(); err == nil {
		err := state.UpdateAfterSuccessDepsCheck(repoRoot)
		if err != nil {
			log.Errorf("Failed to update state AfterSuccessDepsCheck: %v", err)
		}
	}

	return installed, nil
}

// isPermissionDenied inspects exit codes and stderr text across OS types.
func isPermissionDenied(exitCode int, stderr string) bool {
	stderrLower := strings.ToLower(stderr)

	if strings.Contains(stderrLower, "permission denied") ||
		strings.Contains(stderrLower, "operation not permitted") ||
		strings.Contains(stderrLower, "access is denied") ||
		strings.Contains(stderrLower, "requires root privileges") ||
		strings.Contains(stderrLower, "unauthorizedaccessexception") {
		return true
	}

	// Unix-like systems (Linux & macOS) return 126 when a file lacks execute permissions
	if (runtime.GOOS == "linux" || runtime.GOOS == "darwin") && exitCode == 126 {
		return true
	}

	return false
}

// extractFailedManager heuristically identifies the package/version manager
// referenced in cmd (e.g. "brew" in "brew install uv"). Returns "" if none found.
func extractFailedManager(cmd string) string {
	for _, m := range knownManagers {
		if strings.Contains(cmd, m) {
			return m
		}
	}

	return ""
}

// buildInstallFailureMsg composes the user-facing failure message for a failed install.
// env and goos are injectable for testing.
func buildInstallFailureMsg(prerequisite tools.Prerequisite, exitCode int, permDenied bool, env map[string]bool, goos string) string {
	toolName := prerequisite.Name

	installCmd, _ := prerequisite.PlatformInstallCommand()

	var sb strings.Builder

	if permDenied {
		fmt.Fprintf(&sb, "✗ %s install failed (exit code %d — permission denied)\n", toolName, exitCode)
	} else {
		fmt.Fprintf(&sb, "✗ %s install failed (exit code %d)\n", toolName, exitCode)
	}

	fmt.Fprintf(&sb, TAB+"Tried: %s\n", installCmd)

	if tip := buildInstallTip(prerequisite, permDenied, env, goos); tip != "" {
		fmt.Fprintf(&sb, "%s\n", tip)
	}

	fmt.Fprintf(&sb, TAB+"Raw command if you want to retry: %s\n", installCmd)

	if prerequisite.URL != "" {
		fmt.Fprintf(&sb, TAB+"Refer to %s for manual installation instructions.\n", prerequisite.URL)
	}

	return sb.String()
}

// buildInstallTip returns the optional tip line for buildInstallFailureMsg, or "" when
// no actionable suggestion is available.
func buildInstallTip(prerequisite tools.Prerequisite, permDenied bool, env map[string]bool, goos string) string {
	if permDenied {
		switch goos {
		case "windows":
			return TAB + "Tip: This action requires Administrator privileges. Please restart your terminal/tool as Administrator."
		case "darwin", "linux":
			return TAB + "Tip: This action requires root privileges. Please re-run this tool using 'sudo'."
		default:
			return TAB + "Tip: Administrative or root privileges are required to perform this action."
		}
	}

	toolKey := prerequisite.Key
	if toolKey == "" {
		toolKey = NormalizeToolName(prerequisite.Name)
	}

	if toolKey == "" {
		return ""
	}

	installCmd, _ := prerequisite.PlatformInstallCommand()
	failedMgr := extractFailedManager(installCmd)

	strategy := selectInstallStrategy(toolKey, failedMgr, env)
	if strategy == nil {
		return ""
	}

	return strategy.withVersion(prerequisite.MinimumVersion).getStrategyTip(goos)
}

// ExecuteShLine executes shellCmd via sh -c, streaming stdout and stderr
// to w in real time. Handles pipe-based commands (e.g. curl ... | sh).
// Returns the process exit code; a non-nil error indicates the process could
// not be started (not a non-zero exit).
func ExecuteShLine(shellCmd string, w io.Writer) (int, error) {
	cmd := exec.Command("sh", "-c", shellCmd)

	cmd.Stdout = w
	cmd.Stderr = w

	err := cmd.Run()
	if err == nil {
		return 0, nil
	}

	var exitErr *exec.ExitError

	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}

	return 1, err
}
