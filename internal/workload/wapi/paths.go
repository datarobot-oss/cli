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

// Filenames and directory layout for the .wapi/ state directory.
const (
	DirName           = ".wapi"
	ConfigFile        = "config.json"
	ManifestFile      = "manifest.json"
	HistoryFile       = "history.log"
	HistoryBackupFile = "history.log.1"
	GitignoreFile     = ".gitignore"
	WapiignoreFile    = ".wapiignore"
)

// Inline-literal constants kept at package scope so their meaning is obvious
// to readers (and so no one has to guess at the meaning of `"*\n"`).
const gitignoreContents = "*\n"

// ManifestVersion is the current schema version written into manifest.json.
const ManifestVersion = 1

// HistoryRotateBytes is the size threshold at which history.log is rotated
// to history.log.1 (keeping a single backup). See design spec §7.1.
const HistoryRotateBytes int64 = 1 << 20

func wapiDir(projectDir string) string {
	return filepath.Join(projectDir, DirName)
}

func configPath(projectDir string) string {
	return filepath.Join(wapiDir(projectDir), ConfigFile)
}

func manifestPath(projectDir string) string {
	return filepath.Join(wapiDir(projectDir), ManifestFile)
}

func historyPath(projectDir string) string {
	return filepath.Join(wapiDir(projectDir), HistoryFile)
}

func historyBackupPath(projectDir string) string {
	return filepath.Join(wapiDir(projectDir), HistoryBackupFile)
}

func gitignorePath(projectDir string) string {
	return filepath.Join(wapiDir(projectDir), GitignoreFile)
}

// wapiignorePath returns the path of the project-root .wapiignore, which
// lives alongside the user's code rather than inside .wapi/ (see design
// spec §6.4).
func wapiignorePath(projectDir string) string {
	return filepath.Join(projectDir, WapiignoreFile)
}

// Exists reports whether the project directory contains a .wapi/ directory.
// Returns false if .wapi exists as a file rather than a directory.
func Exists(projectDir string) bool {
	return fsutil.DirExists(wapiDir(projectDir))
}
