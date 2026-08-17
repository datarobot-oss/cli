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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/datarobot/cli/internal/fsutil"
)

// LegacyDirName is the state directory older CLIs kept at the project root.
// Nothing reads or relocates it any more, so a project holding one is
// unlinked. The name survives only so commands can recognise that situation
// and explain it: the directory carries a .gitignore of "*", which makes it
// invisible to git status, and the artifact id inside is the one thing needed
// to link the project again. Everything in this file can go once projects
// written by those CLIs have aged out.
const LegacyDirName = ".wapi"

// LegacyState reports whether projectDir holds state at the old root
// location, along with the artifact id recorded there when it can be read. An
// unreadable or malformed config still counts as found: the directory being
// there is what changes the advice a command should give, and a user whose
// config will not parse needs telling more than one whose config will.
func LegacyState(projectDir string) (artifactID string, found bool) {
	dir := filepath.Join(projectDir, LegacyDirName)
	if !fsutil.DirExists(dir) {
		return "", false
	}

	data, err := os.ReadFile(filepath.Join(dir, configFile))
	if err != nil {
		return "", true
	}

	var cfg Config

	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", true
	}

	return cfg.ArtifactID, true
}

// NotLinkedError is the error to return when projectDir has no state
// directory. State at the old location gets a message naming it and the
// artifact it recorded, because the bare hint would send a user who has been
// syncing for months off to link again with no idea why, and no clue that the
// id they need is sitting in a gitignored file at their project root.
func NotLinkedError(projectDir string) error {
	id, found := LegacyState(projectDir)
	if !found {
		return errors.New("not linked: run 'dr artifact code init <artifact-id>' first")
	}

	relink := "<artifact-id>"
	if id != "" {
		relink = id
	}

	return fmt.Errorf(
		"not linked: %s/ holds state written by an older CLI, which is no longer read. "+
			"Run 'dr artifact code init %s' to link this project again, then delete %s/",
		LegacyDirName, relink, LegacyDirName)
}

// LegacyStateNotice is a one-line warning for a command that is about to
// succeed while state is still sitting at the old location, empty when there
// is none so callers can pass it straight through. Linking a project again
// leaves that directory orphaned, and a user is owed a sentence saying it is
// no longer read rather than being left to wonder which of the two counts.
func LegacyStateNotice(projectDir string) string {
	if _, found := LegacyState(projectDir); !found {
		return ""
	}

	return fmt.Sprintf(
		"Note: %s/ holds state written by an older CLI. It is no longer read and can be deleted.",
		LegacyDirName)
}
