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
	"os"
	"path/filepath"

	"github.com/datarobot/cli/internal/fsutil"
	"github.com/datarobot/cli/internal/log"
)

// EnsureMigrated moves a project's state out of the legacy root .wapi/ and
// into .datarobot/wapi/. Call it before any command that touches state.
//
// It is silent and cannot fail. The state directory is machine-managed and
// undocumented, so relocating it is housekeeping the user cannot act on. A
// move that cannot be completed leaves the legacy directory alone and Dir
// keeps resolving there, so the command carries on either way.
func EnsureMigrated(projectDir string) {
	legacy := legacyDir(projectDir)
	if !fsutil.DirExists(legacy) {
		return
	}

	current := filepath.Join(projectDir, RootDirName, StateDirName)

	// Both present means someone re-linked by hand. The current location wins;
	// never merge two trees.
	if fsutil.DirExists(current) {
		log.Debug("state dir migration: skipped, both locations present", "legacy", legacy, "current", current)

		return
	}

	if err := os.MkdirAll(filepath.Dir(current), 0o755); err != nil {
		log.Debug("state dir migration: create parent failed, using legacy location", "dir", filepath.Dir(current), "err", err)

		return
	}

	if err := os.Rename(legacy, current); err != nil {
		log.Debug("state dir migration: rename failed, using legacy location", "from", legacy, "to", current, "err", err)

		return
	}

	log.Debug("state dir migration: moved", "from", legacy, "to", current)
}
