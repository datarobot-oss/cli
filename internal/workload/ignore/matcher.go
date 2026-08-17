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

package ignore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gitignore "github.com/sabhiram/go-gitignore"
)

const wapiignoreFile = ".wapiignore"

// systemExcludes are always-ignored paths, not overridable by .wapiignore.
//
// The state directory is listed at its full path, not as a bare ".datarobot":
// entries match as prefixes, so a bare one would also exclude the CLI's own
// tool state under .datarobot/cli/, which syncs today. ".wapi" is where older
// CLIs kept that state; nothing reads it any more, and it stays listed so a
// leftover one is never uploaded as project source. That matters more than it
// looks: such a directory can hold a .rollback tree containing the only copy
// of files a crashed sync overwrote.
//
// The manifest is excluded because a deploy rewrites it: `up` writes the new
// workload's id into it after the sync has run. Uploaded, it would differ from
// what was synced the moment the deploy finished, so the next run would find a
// changed file, rebuild, and roll a workload nobody had touched, for ever. It
// describes how to deploy the code rather than being part of it, and the
// container has no use for it.
//
// Entries must be lowercase: matchesSystemExclude folds the path it is given.
var systemExcludes = []string{".datarobot/workload", ".wapi", ".git", ".gitignore", ".datarobot.yaml"}

// Matcher decides whether a path is excluded from sync. Match is safe for
// concurrent use after New.
type Matcher struct {
	user *gitignore.GitIgnore // nil when the user has no .wapiignore
}

// New loads .wapiignore from projectDir if present. A missing file is
// fine — only the hardcoded system excludes apply.
func New(projectDir string) (*Matcher, error) {
	path := filepath.Join(projectDir, wapiignoreFile)

	gi, err := gitignore.CompileIgnoreFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Matcher{}, nil
		}

		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	return &Matcher{user: gi}, nil
}

// FromLines builds a Matcher from in-memory pattern lines. Empty/nil
// means "system excludes only".
func FromLines(lines []string) *Matcher {
	if len(lines) == 0 {
		return &Matcher{}
	}

	return &Matcher{user: gitignore.CompileIgnoreLines(lines...)}
}

// Match reports whether relPath should be excluded. isDir lets
// directory-only patterns ("build/") prune subtrees.
func (m *Matcher) Match(relPath string, isDir bool) bool {
	if relPath == "" {
		return false
	}

	if matchesSystemExclude(relPath) {
		return true
	}

	if m.user == nil {
		return false
	}

	if m.user.MatchesPath(relPath) {
		return true
	}

	// Directory-only patterns need a trailing slash to match in go-gitignore.
	if isDir {
		return m.user.MatchesPath(relPath + "/")
	}

	return false
}

// matchesSystemExclude reports whether relPath is or lives inside a
// system-excluded directory.
//
// The comparison folds case because macOS and Windows preserve case without
// distinguishing it: a project that already holds a differently-cased
// .Datarobot/ gets the state directory created inside it, and an exact-match
// exclude would then let config.json and manifest.json sync to the remote. The
// cost is that a case-sensitive filesystem also excludes a genuine .Git or
// .DataRobot, which is not a directory anyone keeps alongside the real ones.
func matchesSystemExclude(relPath string) bool {
	lowered := strings.ToLower(relPath)

	for _, name := range systemExcludes {
		if lowered == name || strings.HasPrefix(lowered, name+"/") {
			return true
		}
	}

	return false
}
