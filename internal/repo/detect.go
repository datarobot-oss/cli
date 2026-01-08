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
	"path/filepath"
)

// FindRepoRoot walks up the directory tree from the current directory looking for
// a .datarobot/cli folder to determine if we're inside a DataRobot repository.
// It stops searching when it reaches the user's home directory or finds a .git folder.
// Returns the path to the repository root if found, or an empty string if not found.
func FindRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	currentDir := cwd

	for {
		// Check if .datarobot/cli exists in current directory
		datarobotCLIPath := filepath.Join(currentDir, DataRobotRepoPath)
		if info, err := os.Stat(datarobotCLIPath); err == nil && info.IsDir() {
			return currentDir, nil
		}

		// Check if we've reached the home directory
		if currentDir == homeDir {
			return "", nil
		}

		// Check if .git folder exists (stop searching beyond git repo boundary)
		gitPath := filepath.Join(currentDir, ".git")
		if info, err := os.Stat(gitPath); err == nil && info.IsDir() {
			// We found a .git folder but no .datarobot/cli, so not in a DataRobot repo
			return "", nil
		}

		// Move up one directory
		parentDir := filepath.Dir(currentDir)

		// Check if we've reached the root of the filesystem
		if parentDir == currentDir {
			return "", nil
		}

		currentDir = parentDir
	}
}

// IsInRepo checks if the current directory is inside a DataRobot repository
// by looking for a .datarobot/cli folder in the current or parent directories.
func IsInRepo() bool {
	repoRoot, err := FindRepoRoot()
	return err == nil && repoRoot != ""
}

func IsInRepoRoot() bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}

	repoRoot, err := FindRepoRoot()

	return err == nil && repoRoot == cwd
}
