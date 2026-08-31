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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemExcludes_AlwaysApply(t *testing.T) {
	m := FromLines(nil)

	cases := []struct {
		path string
		want bool
	}{
		{".datarobot/workload", true},
		{".datarobot/workload/config.json", true},
		{".wapi", true},             // legacy, still excluded pre-migration
		{".wapi/config.json", true}, // legacy, still excluded pre-migration
		{".git", true},
		{".git/HEAD", true},
		{".gitignore", true},
		{"agent.py", false},
		{".drignore", false},   // user-editable, lives at root
		{".wapiignore", false}, // same, under the name it used to have
		// The manifest is rewritten by a deploy after the sync, so uploading it
		// would make the tree differ from what was synced on every later run.
		{".datarobot.yaml", true},
		{".datarobot/cli", false}, // the CLI's own tool state is not sync state
		{".datarobot/cli/state.yaml", false},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			assert.Equal(t, tc.want, m.Match(tc.path, false))
		})
	}
}

// The engine's own *.LOCAL backups are excluded from the sync walk at any
// depth, so a copy of an overwritten file is never uploaded as new content and
// the next sync does not see it (RAPTOR-19348).
func TestMatch_ExcludesLocalBackups(t *testing.T) {
	m := FromLines(nil)

	cases := []struct {
		path string
		want bool
	}{
		{"app.py.LOCAL.20260102T030405Z", true},
		{"src/nested/config.json.LOCAL.20260102T030405Z", true},
		{"app.py", false},
		{"notes.LOCAL.txt", false},     // ".LOCAL." but no timestamp: a real file
		{"data.LOCAL.2026.csv", false}, // not the engine's stamp shape
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			assert.Equal(t, tc.want, IsBackupCopy(tc.path), "IsBackupCopy")
			assert.Equal(t, tc.want, m.Match(tc.path, false), "Match")
		})
	}
}

// On macOS and Windows the state directory can end up inside a pre-existing
// .Datarobot/ that the project already had: the filesystem preserves that
// casing while treating it as the same directory, so the walk reports a path no
// exact-match exclude would catch and the state files would sync.
func TestSystemExcludes_FoldCase(t *testing.T) {
	m := FromLines(nil)

	for _, p := range []string{
		".Datarobot/workload/config.json",
		".DATAROBOT/WORKLOAD",
		".datarobot/Workload/manifest.json",
		".WAPI/config.json",
		".Git/HEAD",
	} {
		t.Run(p, func(t *testing.T) {
			assert.True(t, m.Match(p, false))
		})
	}

	// Folding must not swallow neighbours that merely share a prefix.
	assert.False(t, m.Match(".Datarobot/cli/state.yaml", false))
	assert.True(t, m.Match(".Datarobot.yaml", false), "the manifest is excluded whatever its casing")
}

func TestSystemExcludes_NotOverridable(t *testing.T) {
	// User explicitly tries to un-ignore the state dir via negation. Must
	// still be excluded — the system excludes win.
	m := FromLines([]string{"!.datarobot/workload", "!.wapi", "!.git"})

	assert.True(t, m.Match(".datarobot/workload", true))
	assert.True(t, m.Match(".datarobot/workload/manifest.json", false))
	assert.True(t, m.Match(".wapi", true))
	assert.True(t, m.Match(".git", true))
}

func TestUserPatterns(t *testing.T) {
	m := FromLines([]string{
		"__pycache__",
		"*.pyc",
		".env",
		"*.LOCAL.*",
		"build/",
		"!keep.me",
	})

	cases := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{path: "agent.py", want: false},
		{path: "agent.pyc", want: true},
		{path: "src/utils.pyc", want: true},
		{path: "src/__pycache__", isDir: true, want: true},
		{path: "src/__pycache__/foo.pyc", want: true},
		{path: ".env", want: true},
		{path: "agent.py.LOCAL.20260410T143052Z", want: true},
		{path: "build", isDir: true, want: true},
		{path: "build/dist.tar", want: true},
		{path: "keep.me", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			assert.Equal(t, tc.want, m.Match(tc.path, tc.isDir))
		})
	}
}

func TestNew_NoIgnoreFile(t *testing.T) {
	dir := t.TempDir()

	m, err := New(dir)
	require.NoError(t, err)

	// System excludes still apply.
	assert.True(t, m.Match(".datarobot/workload/foo", false))
	// Without a user file, regular paths pass through.
	assert.False(t, m.Match("agent.py", false))

	assert.Empty(t, m.Source())
	assert.Empty(t, m.Notice())
}

func TestNew_CurrentName(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, FileName, "*.tmp\n")

	m, err := New(dir)
	require.NoError(t, err)

	assert.True(t, m.Match("scratch.tmp", false))
	assert.False(t, m.Match("agent.py", false))

	assert.Equal(t, FileName, m.Source())
	assert.Empty(t, m.Notice(), "the current name is not worth a word")
	assert.Empty(t, m.ShadowWarning())
}

// A project set up before the rename has a committed .wapiignore and nothing
// else. Skipping it would upload the .venv and .env it was written to keep out,
// so it still filters, and the user is told once that the name is going away.
func TestNew_FallsBackToLegacyName(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, LegacyFileName, "*.tmp\n")

	m, err := New(dir)
	require.NoError(t, err)

	assert.True(t, m.Match("scratch.tmp", false))
	assert.False(t, m.Match("agent.py", false))

	assert.Equal(t, LegacyFileName, m.Source())

	notice := m.Notice()
	assert.Contains(t, notice, LegacyFileName, "say which file was read")
	assert.Contains(t, notice, FileName, "and what to rename it to")

	assert.Empty(t, m.ShadowWarning(), "nothing is being shadowed by one file")
}

// Mid-rename, both files sit at the root. The current name wins outright and
// the legacy one is not applied, which the disjoint patterns prove: only the
// .drignore rule matches.
func TestNew_CurrentNameWinsOverLegacy(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, FileName, "*.new\n")
	writeIgnore(t, dir, LegacyFileName, "*.old\n")

	m, err := New(dir)
	require.NoError(t, err)

	assert.True(t, m.Match("scratch.new", false))
	assert.False(t, m.Match("scratch.old", false), "the fallback must not apply")

	assert.Equal(t, FileName, m.Source())
	assert.Empty(t, m.Notice(), "the file in effect has the current name")

	warning := m.ShadowWarning()
	assert.Contains(t, warning, FileName, "say which one is in effect")
	assert.Contains(t, warning, LegacyFileName, "and which one is inert")
}

// An empty .drignore beside a tuned .wapiignore is the shape that costs the
// most and looks the least like a mistake: `touch .drignore` in response to
// the deprecation line retires every pattern the user wrote. It compiles to a
// valid matcher with no rules, so nothing downstream can tell it apart from a
// deliberate "ignore nothing" without being told.
func TestNew_EmptyCurrentFileShadowingLegacyIsWarnedAbout(t *testing.T) {
	for name, body := range map[string]string{
		"empty":            "",
		"comments only":    "# nothing here\n",
		"blank lines only": "\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeIgnore(t, dir, FileName, body)
			writeIgnore(t, dir, LegacyFileName, ".env\n.venv\n")

			m, err := New(dir)
			require.NoError(t, err)

			assert.False(t, m.Match(".env", false), "the legacy patterns are genuinely inert")
			assert.NotEmpty(t, m.ShadowWarning(), "which is exactly why it has to be said")
		})
	}
}

// Locate is what the writer side asks before dropping a starter template, so
// it has to agree with which file New would read.
func TestLocate(t *testing.T) {
	t.Run("neither", func(t *testing.T) {
		assert.Empty(t, Locate(t.TempDir()))
	})

	t.Run("legacy only", func(t *testing.T) {
		dir := t.TempDir()
		writeIgnore(t, dir, LegacyFileName, "*.tmp\n")

		assert.Equal(t, filepath.Join(dir, LegacyFileName), Locate(dir))
	})

	t.Run("both", func(t *testing.T) {
		dir := t.TempDir()
		writeIgnore(t, dir, FileName, "*.new\n")
		writeIgnore(t, dir, LegacyFileName, "*.old\n")

		assert.Equal(t, filepath.Join(dir, FileName), Locate(dir), "same precedence as New")
	})

	// A directory is not an ignore file, and neither is a link to nothing.
	// Locate has to answer these the way New does: calling them present would
	// have the writer side skip the starter template while New found nothing
	// to read, leaving the project filtering by nothing at all.
	t.Run("a directory at the name is not a file", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, FileName), 0o755))

		assert.Empty(t, Locate(dir))
	})

	t.Run("a dangling symlink is not a file", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, FileName)))

		assert.Empty(t, Locate(dir), "New cannot read it either, so neither may claim it is there")
	})
}

// A symlink to a file that is not checked out on this machine is the shape
// that used to fall between Locate and New: one saw the link, the other read
// through it and found nothing, so the project ended up with no starter
// template and no patterns, silently.
func TestNew_AndLocateAgreeOnADanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, FileName)))
	writeIgnore(t, dir, LegacyFileName, ".env\n")

	m, err := New(dir)
	require.NoError(t, err)

	assert.Equal(t, LegacyFileName, m.Source(), "the readable file is the one in effect")
	assert.True(t, m.Match(".env", false))
	assert.Equal(t, filepath.Join(dir, LegacyFileName), Locate(dir), "and Locate names the same one")
}

// An unreadable current file must not fall through to the legacy one. Silently
// filtering by a different set of patterns than the one the user is editing is
// how a .env reaches the remote; failing is recoverable, that is not.
func TestNew_UnreadableFileInEffectFails(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, LegacyFileName, "*.tmp\n")

	// A directory under the current name is unreadable as a file everywhere,
	// unlike a chmod that root and Windows both ignore.
	require.NoError(t, os.Mkdir(filepath.Join(dir, FileName), 0o755))

	m, err := New(dir)
	require.Error(t, err)
	assert.Nil(t, m)
	assert.Contains(t, err.Error(), FileName)
	assert.Equal(t, 1, strings.Count(err.Error(), FileName), "the path is named once, not wrapped over itself")
}

// The opposite order must not fail. A leftover the run would not have applied
// has no say in whether the run happens: its patterns are inert either way, so
// reading it is work that can only produce an error nobody needed. A directory
// is not an ignore file, so there is nothing to warn about either.
func TestNew_UnreadableShadowedEntryIsIgnored(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, FileName, "*.tmp\n")
	require.NoError(t, os.Mkdir(filepath.Join(dir, LegacyFileName), 0o755))

	m, err := New(dir)
	require.NoError(t, err, "a leftover that would not have applied must not fail the sync")

	assert.Equal(t, FileName, m.Source())
	assert.True(t, m.Match("scratch.tmp", false), "the good file still applies")
	assert.Empty(t, m.ShadowWarning(), "a directory is not a second ignore file")
}

func writeIgnore(t *testing.T, dir, name, body string) {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
}
