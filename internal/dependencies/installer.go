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
	"errors"
	"fmt"
	"io"
	"os/exec"

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

		exitCode, err := ExecuteShLine(installCmd, w)
		if err != nil {
			log.Debug("deps: install failed to start", "name", prerequisite.Name, "err", err)

			return installed, fmt.Errorf("failed to start install for %q: %w\n  command: %s", prerequisite.Name, err, installCmd)
		}

		if exitCode != 0 {
			log.Debug("deps: install exited non-zero", "name", prerequisite.Name, "exit_code", exitCode)

			return installed, fmt.Errorf("install failed for %q (exit code %d)\n  Please run manually: %s\n  Or check %s", prerequisite.Name, exitCode, installCmd, prerequisite.URL)
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
