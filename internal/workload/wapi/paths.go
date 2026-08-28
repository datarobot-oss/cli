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

package wapi

import (
	"path/filepath"

	"github.com/datarobot/cli/internal/fsutil"
	"github.com/datarobot/cli/internal/workload/ignore"
)

// Exported names used by external callers (tests, sync commands).
const (
	// RootDirName is the single directory a project gives DataRobot. The CLI
	// already keeps its tool state under it, so sync state joins it rather
	// than claiming a second dot-entry at the project root.
	RootDirName = ".datarobot"

	// StateDirName is the sync state directory, nested inside RootDirName.
	// It is named after the product surface rather than the package, so the
	// path a user sees does not carry an internal codename.
	StateDirName = "workload"

	// LegacyDirName is where sync state lived before it moved under
	// RootDirName. EnsureMigrated relocates it.
	LegacyDirName = ".wapi"

	// RollbackDirName is the sync engine's per-run backup tree.
	RollbackDirName = ".rollback"

	HistoryFile      = "history.log"
	ManifestVersion  = 1
	CheckoutsDirName = ".checkouts"
)

// Internal layout constants. Stay private so consumers go through the
// helpers below rather than reconstructing paths by hand.
const (
	configFile        = "config.json"
	manifestFile      = "manifest.json"
	historyBackupFile = "history.log.1"
	gitignoreFile     = ".gitignore"

	gitignoreContents = "*\n"

	// Size threshold at which history.log rotates to history.log.1
	// (only one backup is retained).
	historyRotateBytes int64 = 1 << 20
)

// Dir is the directory holding this project's machine-managed sync state:
// <projectDir>/.datarobot/workload, falling back to a legacy <projectDir>/.wapi
// that EnsureMigrated has not moved yet. The fallback is a stat rather than
// cached state, so a migration that cannot complete degrades to reading the
// old location instead of failing the command. It can go once the workload
// feature gate is removed.
func Dir(projectDir string) string {
	current := filepath.Join(projectDir, RootDirName, StateDirName)
	if fsutil.DirExists(current) {
		return current
	}

	if legacy := legacyDir(projectDir); fsutil.DirExists(legacy) {
		return legacy
	}

	return current
}

func legacyDir(projectDir string) string {
	return filepath.Join(projectDir, LegacyDirName)
}

// RollbackDir is the sync engine's backup tree for a single run. Callers go
// through this rather than joining the name themselves.
func RollbackDir(projectDir string) string {
	return filepath.Join(Dir(projectDir), RollbackDirName)
}

// StaleRollbackDirs lists every place a rollback tree from a crashed sync
// could be sitting, current location first. Recovery looks wider than
// RollbackDir alone because a sync that crashed before the state directory
// moved leaves its backups under the legacy path, where nothing will relocate
// them if a current directory also exists. They are the only copy of the
// user's overwritten files.
func StaleRollbackDirs(projectDir string) []string {
	current := filepath.Join(projectDir, RootDirName, StateDirName, RollbackDirName)
	legacy := filepath.Join(legacyDir(projectDir), RollbackDirName)

	return []string{current, legacy}
}

// ConfigPath is the project's config.json, exported so commands can name the
// real file when wrapping a read error rather than hard-coding a path.
func ConfigPath(projectDir string) string {
	return configPath(projectDir)
}

func configPath(projectDir string) string {
	return filepath.Join(Dir(projectDir), configFile)
}

func manifestPath(projectDir string) string {
	return filepath.Join(Dir(projectDir), manifestFile)
}

func historyPath(projectDir string) string {
	return filepath.Join(Dir(projectDir), HistoryFile)
}

func historyBackupPath(projectDir string) string {
	return filepath.Join(Dir(projectDir), historyBackupFile)
}

func gitignorePath(projectDir string) string {
	return filepath.Join(Dir(projectDir), gitignoreFile)
}

// The ignore file lives at the project root, not inside the state directory:
// it is authored and committed by the user, the same convention as .gitignore.
// The name comes from the package that reads it so the two cannot drift.
func ignorePath(projectDir string) string {
	return filepath.Join(projectDir, ignore.FileName)
}

// Exists reports whether the project is linked, at either the current or the
// legacy location. False if the path is a file rather than a directory.
func Exists(projectDir string) bool {
	return fsutil.DirExists(Dir(projectDir))
}

// CheckoutsDir is the parent directory holding read-only version snapshots
// produced by `dr artifact code checkout`. Each snapshot is a sub-directory
// named after the full catalog version ID.
func CheckoutsDir(projectDir string) string {
	return filepath.Join(Dir(projectDir), CheckoutsDirName)
}

// CheckoutDir is the snapshot directory for a single version, identified by
// its full catalog version ID.
func CheckoutDir(projectDir, versionID string) string {
	return filepath.Join(CheckoutsDir(projectDir), versionID)
}
