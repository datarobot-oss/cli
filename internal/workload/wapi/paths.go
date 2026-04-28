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

// Exported names used by external callers (tests, future c2w commands).
const (
	DirName         = ".wapi"
	HistoryFile     = "history.log"
	ManifestVersion = 1
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

func wapiDir(projectDir string) string {
	return filepath.Join(projectDir, DirName)
}

func configPath(projectDir string) string {
	return filepath.Join(wapiDir(projectDir), configFile)
}

func manifestPath(projectDir string) string {
	return filepath.Join(wapiDir(projectDir), manifestFile)
}

func historyPath(projectDir string) string {
	return filepath.Join(wapiDir(projectDir), HistoryFile)
}

func historyBackupPath(projectDir string) string {
	return filepath.Join(wapiDir(projectDir), historyBackupFile)
}

func gitignorePath(projectDir string) string {
	return filepath.Join(wapiDir(projectDir), gitignoreFile)
}

// .wapiignore lives at the project root (not inside .wapi/) so it is visible
// in the IDE and version-controllable, the same convention as .gitignore.
func wapiignorePath(projectDir string) string {
	return filepath.Join(projectDir, wapiignoreFile)
}

// Exists reports whether the project directory contains a .wapi/ directory.
// Returns false if .wapi exists as a file rather than a directory.
func Exists(projectDir string) bool {
	return fsutil.DirExists(wapiDir(projectDir))
}
