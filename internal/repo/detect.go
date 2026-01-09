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

package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/datarobot/cli/internal/fsutil"
)

// FindRepoRoot walks up the directory tree from the current directory looking for
// a .datarobot/answers folder to determine if we're inside a DataRobot repository.
// It stops searching when it reaches the user's home directory or finds a .git folder.
// Returns the path to the repository root if found, or an empty string if not found.
func FindRepoRoot() (string, error) {
	currentDir, err := gitTopLevel()
	if err != nil {
		return "", err
	}

	for {
		// Check if .datarobot/answers exists in current directory
		if detectTemplate(currentDir) {
			return currentDir, nil
		}

		// Check if we are in submodule
		superDir, err := gitSuperProject()
		if err != nil {
			return "", err
		}

		if superDir == "" {
			return "", nil
		}

		currentDir = superDir
	}
}

// detectTemplate checks if .datarobot/answers exists in dir directory
func detectTemplate(dir string) bool {
	return fsutil.DirExists(filepath.Join(dir, DataRobotTemplateDetectPath))
}

func gitTopLevel() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

func gitSuperProject() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-superproject-working-tree")

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

// IsInRepo checks if the current directory is inside a DataRobot repository
// by looking for a .datarobot/answers folder in the current or parent directories.
func IsInRepo() bool {
	repoRoot, err := FindRepoRoot()
	if err != nil {
		return false
	}

	return repoRoot != ""
}

func IsInRepoRoot() bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}

	repoRoot, err := FindRepoRoot()
	if err != nil {
		return false
	}

	return repoRoot == cwd
}
