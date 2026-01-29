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
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/fsutil"
)

// FindRepoRoot walks up the directory tree from the current directory looking for
// a .datarobot/answers folder to determine if we're inside a DataRobot repository.
// It stops searching when it reaches the user's home directory or finds a .git folder.
// Returns the path to the repository root if found, or an empty string if not found.
func FindRepoRoot() (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	for {
		// Check if .datarobot/answers exists in current directory
		if detectTemplate(currentDir) {
			return currentDir, nil
		}

		// Check if we've reached the home directory
		if currentDir == homeDir {
			return "", errors.New("reached home directory")
		}

		// Move up one directory
		parentDir := filepath.Dir(currentDir)

		// Check if we've reached the root of the filesystem
		if parentDir == currentDir {
			return "", errors.New("reached filesystem root")
		}

		currentDir = parentDir
	}
}

// detectTemplate checks if .datarobot/answers or .datarobot/cli exists in dir directory
func detectTemplate(dir string) bool {
	answersDirPresent := fsutil.DirExists(filepath.Join(dir, DataRobotTemplateDetectAnswersPath))
	if answersDirPresent {
		log.Debugf("Directory %s exists, treating %s as template", DataRobotTemplateDetectAnswersPath, dir)
		return true
	}

	entries, err := os.ReadDir(filepath.Join(dir, DataRobotTemplateDetectCliPath))
	if err != nil {
		return false
	}

	if len(entries) == 0 {
		log.Debugf("Empty CLI configuration directory %s exists, treating %s as template", DataRobotTemplateDetectCliPath, dir)
	}

	// Older versions were incorrectly creating state.yaml file outside of template directories
	// return true if any file other than state.yaml exists in .datarobot/cli
	cliConfigDirPresent := slices.ContainsFunc(entries, func(entry os.DirEntry) bool {
		return entry.Name() != "state.yaml"
	})

	if cliConfigDirPresent {
		log.Debugf("CLI configuration files present, treating %s as template", dir)
		return true
	}

	return false
}

// IsInRepo checks if the current directory is inside a DataRobot repository
// by looking for a .datarobot/answers folder in the current or parent directories.
func IsInRepo() bool {
	_, err := FindRepoRoot()
	return err == nil
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
