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

	"github.com/datarobot/cli/internal/fsutil"
	gitignore "github.com/sabhiram/go-gitignore"
)

const (
	// FileName is the user-authored ignore file at the project root. It is
	// exported so the package that writes the starter template names the same
	// file this one reads, rather than keeping a second copy of the string.
	FileName = ".drignore"

	// LegacyFileName is the name FileName replaced. It is read only when
	// FileName is absent: a project set up before the rename has a committed
	// one, and ignoring it would upload the .venv, node_modules and .env it
	// was written to keep out.
	LegacyFileName = ".wapiignore"
)

// systemExcludes are always-ignored paths, not overridable by .drignore.
//
// The state directory is listed at its full path, not as a bare ".datarobot":
// entries match as prefixes, so a bare one would also exclude the CLI's own
// tool state under .datarobot/cli/, which syncs today. The legacy ".wapi"
// stays listed so an un-migrated project does not upload its state.
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
	user *gitignore.GitIgnore // nil when the user has no ignore file

	// source is the base filename the patterns came from, empty when none
	// was loaded. Kept so callers can name the file the user actually has
	// instead of assuming the current name.
	source string

	// shadowed is a candidate that lost to source, empty when only one was
	// present. Its patterns are not applied and the user is told so.
	shadowed string
}

// Locate returns the path New would take its patterns from, or "" when the
// project has neither name. The package that writes the starter template asks
// this rather than testing for a filename itself, so the two cannot drift.
//
// It uses the same file test as the rest of the CLI, which means it agrees
// with New on the awkward cases: a directory at one of these names is not an
// ignore file, and neither is a symlink whose target is missing. Answering
// "present" for either would have the writer side skip the template while New
// found nothing to read, leaving the project with no patterns at all.
func Locate(projectDir string) string {
	for _, name := range []string{FileName, LegacyFileName} {
		path := filepath.Join(projectDir, name)
		if fsutil.FileExists(path) {
			return path
		}
	}

	return ""
}

// New loads the project's ignore file, preferring FileName and falling back to
// LegacyFileName when it is absent. A project with neither is fine: only the
// hardcoded system excludes apply.
//
// The newer name wins outright when both are present. Merging two files would
// make the effective pattern set depend on an ordering nobody wrote down, and
// a project that has both is mid-rename, where the new file is the one being
// edited. ShadowWarning reports the loser, because a file at the project root
// that looks like it is filtering the sync and is not needs saying out loud.
//
// An unreadable file in effect fails rather than falling through to the older
// name. Quietly syncing under a different set of patterns than the one the
// user is looking at is how a .env reaches the remote; a failed sync they can
// retry is the better end of that trade. The loser is only ever stat'd, so a
// leftover the run would not have applied cannot fail it either.
func New(projectDir string) (*Matcher, error) {
	gi, err := compileIfPresent(filepath.Join(projectDir, FileName))
	if err != nil {
		return nil, err
	}

	if gi != nil {
		return &Matcher{user: gi, source: FileName, shadowed: shadowedBy(projectDir)}, nil
	}

	gi, err = compileIfPresent(filepath.Join(projectDir, LegacyFileName))
	if err != nil {
		return nil, err
	}

	if gi == nil {
		return &Matcher{}, nil
	}

	return &Matcher{user: gi, source: LegacyFileName}, nil
}

// shadowedBy names the older file sitting beside the one in effect, empty when
// there is none. Existence is all the warning needs, so this does not read it:
// compiling a file whose patterns are inert would be work thrown away, and its
// read errors would fail a sync that was never going to use it.
func shadowedBy(projectDir string) string {
	if fsutil.FileExists(filepath.Join(projectDir, LegacyFileName)) {
		return LegacyFileName
	}

	return ""
}

// compileIfPresent returns nil, nil when path does not exist, so New can try
// the next candidate without treating absence as failure.
//
// A read failure is returned as it comes: os.ReadFile's error is already a
// *PathError naming the operation and the file, so wrapping it here would only
// print the path twice.
func compileIfPresent(path string) (*gitignore.GitIgnore, error) {
	gi, err := gitignore.CompileIgnoreFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, err
	}

	return gi, nil
}

// Source is the base filename the patterns were read from, empty when the
// Matcher has none. Callers writing a message about a pattern use it so the
// advice points at the file that exists on disk.
func (m *Matcher) Source() string { return m.source }

// Notice is a one-line account of the legacy filename still being in use,
// empty in every other case. The file keeps working either way, so this is the
// only signal a user gets that the old name is on its way out.
//
// It carries the coordination caveat because the ignore file is part of the
// synced set: renaming it deletes the old name from the remote, and a
// collaborator on a CLI from before this change reads only that name. The
// person who runs the rename is the one who needs to know, and they are
// reading this rather than the docs.
//
// Printing it is the caller's job, which keeps this package doing no I/O
// beyond finding the ignore file. Same for ShadowWarning. The two are never
// both set: the legacy name is only ever in effect when it is the only one.
func (m *Matcher) Notice() string {
	if m.source != LegacyFileName {
		return ""
	}

	return fmt.Sprintf(
		"Using %s for sync ignore patterns. That name is deprecated: rename the file to %s. "+
			"It syncs with your code, so agree the rename with anyone else deploying this project.",
		LegacyFileName, FileName)
}

// ShadowWarning reports a second ignore file whose patterns are not being
// applied, empty when the project has at most one.
//
// It is separate from Notice, and a warning rather than housekeeping, because
// the two states differ in what they cost. Using the old name costs nothing;
// the patterns still apply. Having both means a set of patterns the user wrote
// is silently inert, and the ways to end up there do not look like mistakes: a
// half-finished rename, or a `touch .drignore` in response to the deprecation
// line. Sync can also create it unaided, since the ignore file is itself part
// of the synced set, so a machine that links to an artifact carrying the other
// name downloads it right next to the one already on disk.
//
// Nothing else in a run would mention it. The first sign otherwise is a .venv
// on the remote.
func (m *Matcher) ShadowWarning() string {
	if m.shadowed == "" {
		return ""
	}

	return fmt.Sprintf(
		"Both %s and %s are present. %s is the one in effect, and the patterns in %s are not applied. "+
			"Merge them into %s and delete %s.",
		FileName, m.shadowed, FileName, m.shadowed, FileName, m.shadowed)
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
