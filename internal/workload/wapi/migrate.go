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
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/datarobot/cli/internal/fsutil"
	"github.com/datarobot/cli/internal/log"
)

// EnsureMigrated moves a project's state out of the legacy root .wapi/ and
// into .datarobot/workload/. Call it before any command that touches state.
//
// It cannot fail. A move that cannot be completed leaves the legacy directory
// alone and Dir keeps resolving there, so the command carries on either way.
//
// The return value is a one-line notice, empty when there is nothing to say.
// The directory is machine-managed but it is not invisible: a user who watches
// one at their project root vanish, or a stale one linger, is owed a sentence
// saying which location is now in use. Printing it is the caller's job, so
// this package keeps doing no I/O beyond the state files themselves.
func EnsureMigrated(projectDir string) string {
	legacy := legacyDir(projectDir)
	if !fsutil.DirExists(legacy) {
		return ""
	}

	current := filepath.Join(projectDir, RootDirName, StateDirName)
	currentName := path.Join(RootDirName, StateDirName)

	// Both present means someone re-linked by hand. The current location wins;
	// never merge two trees.
	if fsutil.DirExists(current) {
		log.Debug("state dir migration: skipped, both locations present", "legacy", legacy, "current", current)

		return fmt.Sprintf("Using %s/ for local state; the legacy %s/ is no longer read and can be deleted.", currentName, LegacyDirName)
	}

	if err := os.MkdirAll(filepath.Dir(current), 0o755); err != nil {
		log.Debug("state dir migration: create parent failed, using legacy location", "dir", filepath.Dir(current), "err", err)

		return unmovedNotice(currentName)
	}

	if err := os.Rename(legacy, current); err != nil {
		log.Debug("state dir migration: rename failed, using legacy location", "from", legacy, "to", current, "err", err)

		return unmovedNotice(currentName)
	}

	log.Debug("state dir migration: moved", "from", legacy, "to", current)

	return fmt.Sprintf("Moved local state from %s/ to %s/.", LegacyDirName, currentName)
}

// unmovedNotice names the location still in use when the move could not be
// made, so a read-only or otherwise stuck project explains itself rather than
// looking like the state directory was ignored.
func unmovedNotice(currentName string) string {
	return fmt.Sprintf("Could not move local state from %s/ to %s/; using %s/ for now.", LegacyDirName, currentName, LegacyDirName)
}
