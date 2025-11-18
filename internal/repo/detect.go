// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

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

// FindGitRoot walks up the directory tree from the given directory looking for
// a .git folder to determine if we're inside any git repository.
// It stops searching when it reaches the user's home directory or filesystem root.
// Returns the path to the git repository root if found, or an empty string if not found.
func FindGitRoot(dir string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	currentDir := absDir

	for {
		// Check if .git exists in current directory
		gitPath := filepath.Join(currentDir, ".git")
		if info, err := os.Stat(gitPath); err == nil && (info.IsDir() || info.Mode().IsRegular()) {
			return currentDir, nil
		}

		// Check if we've reached the home directory
		if currentDir == homeDir {
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

// IsInGitRepo checks if the given directory is inside a git repository
// by looking for a .git folder in the directory or parent directories.
func IsInGitRepo(dir string) bool {
	gitRoot, err := FindGitRoot(dir)
	return err == nil && gitRoot != ""
}
