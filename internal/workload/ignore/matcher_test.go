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
		{".wapiignore", false}, // user-editable, lives at root
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

func TestNew_NoWapiignore(t *testing.T) {
	dir := t.TempDir()

	m, err := New(dir)
	require.NoError(t, err)

	// System excludes still apply.
	assert.True(t, m.Match(".datarobot/workload/foo", false))
	// Without a user file, regular paths pass through.
	assert.False(t, m.Match("agent.py", false))
}

func TestNew_WithWapiignore(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".wapiignore"), []byte("*.tmp\n"), 0o644))

	m, err := New(dir)
	require.NoError(t, err)

	assert.True(t, m.Match("scratch.tmp", false))
	assert.False(t, m.Match("agent.py", false))
}
