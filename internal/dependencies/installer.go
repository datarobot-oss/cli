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

	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/internal/state"
	"github.com/datarobot/cli/internal/tools"
)

func InstallPrerequisites(w io.Writer, prerequisites []tools.Prerequisite) error {
	// Install prerequisites sequentially via ExecuteShLine(), stopping on the first failure
	for _, prerequisite := range prerequisites {
		installCmd, err := prerequisite.PlatformInstallCommand()
		if err != nil {
			return err
		}

		fmt.Fprintf(w, "📦 Installing %s...\n", prerequisite.Name)

		exitCode, err := ExecuteShLine(installCmd, w)
		if err != nil {
			return fmt.Errorf("failed to start install for %q: %w\n  command: %s", prerequisite.Name, err, installCmd)
		}

		if exitCode != 0 {
			return fmt.Errorf("install failed for %q (exit code %d)\n  Please run manually: %s\n  Or check %s", prerequisite.Name, exitCode, installCmd, prerequisite.URL)
		}

		fmt.Fprintf(w, "✅ %s installed\n", prerequisite.Name)
	}

	fmt.Fprintf(w, "\n✅ All dependencies installed successfully.\n")

	// Update state after successful installs.
	// Executed only after all installs succeed to avoid state inconsistency if an install fails.
	// This is necessary to avoid prompting the user to install already installed dependencies on the next run.
	repoRoot, err := repo.FindRepoRoot()
	if err != nil {
		return err
	}

	err = state.UpdateAfterDepsInstall(repoRoot)
	if err != nil {
		return err
	}

	return nil
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
