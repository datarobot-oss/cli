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
	wapiignoreFile    = ".wapiignore"

	gitignoreContents = "*\n"

	// Size threshold at which history.log rotates to history.log.1
	// (only one backup is retained).
	historyRotateBytes int64 = 1 << 20
)

// Dir is the directory holding this project's machine-managed sync state:
// <projectDir>/.datarobot/workload. It names the location whether or not it
// exists yet, so callers creating state and callers reading it agree on one
// path; use Exists to ask whether the project is linked.
func Dir(projectDir string) string {
	return filepath.Join(projectDir, RootDirName, StateDirName)
}

// RollbackDir is the sync engine's backup tree for a single run. Callers go
// through this rather than joining the name themselves. A tree left behind by
// a crashed sync is the only copy of the user's overwritten files, so recovery
// reads the same path rather than reconstructing it.
func RollbackDir(projectDir string) string {
	return filepath.Join(Dir(projectDir), RollbackDirName)
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

// .wapiignore lives at the project root, not inside the state directory: it is
// authored and committed by the user, the same convention as .gitignore.
func wapiignorePath(projectDir string) string {
	return filepath.Join(projectDir, wapiignoreFile)
}

// Exists reports whether the project is linked, meaning its state directory is
// present. False if the path is a file rather than a directory.
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
