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

package fileops

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, root, rel string, body string) {
	t.Helper()

	p := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
}

func relPaths(es []Entry) []string {
	out := make([]string, len(es))

	for i, e := range es {
		out[i] = e.RelPath
	}

	sort.Strings(out)

	return out
}

func TestWalk_Basic(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "agent.py", "x")
	writeFile(t, root, "utils/helper.py", "y")
	writeFile(t, root, "models/bert/config.json", "{}")

	got, err := Walk(root, nil, nil)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"agent.py",
		"utils/helper.py",
		"models/bert/config.json",
	}, relPaths(got))
}

func TestWalk_IgnoreFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "agent.py", "x")
	writeFile(t, root, "secret.env", "TOKEN=abc")

	ignore := func(rel string, isDir bool) bool {
		_ = isDir
		return rel == "secret.env"
	}

	got, err := Walk(root, ignore, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"agent.py"}, relPaths(got))
}

func TestWalk_PrunesIgnoredDirectory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "agent.py", "x")
	writeFile(t, root, "node_modules/foo/index.js", "1")
	writeFile(t, root, "node_modules/bar/index.js", "2")

	calls := 0
	ignore := func(rel string, isDir bool) bool {
		calls++
		return isDir && rel == "node_modules"
	}

	got, err := Walk(root, ignore, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"agent.py"}, relPaths(got))
	// Pruning means we should have called ignore for the node_modules
	// directory but never for any child path inside it.
	assert.Less(t, calls, 8, "ignore should not have been called for every descendant")
}

func TestWalk_SkipsSymlinkAndNotifies(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test skipped on windows")
	}

	root := t.TempDir()
	writeFile(t, root, "agent.py", "x")
	writeFile(t, root, "real/data.bin", "z")
	require.NoError(t, os.Symlink(filepath.Join(root, "real", "data.bin"), filepath.Join(root, "data.lnk")))

	var seen []string

	got, err := Walk(root, nil, func(rel, target string) {
		_ = target

		seen = append(seen, rel)
	})
	require.NoError(t, err)
	// data.lnk is NOT in entries; the underlying real/data.bin IS.
	assert.ElementsMatch(t, []string{"agent.py", "real/data.bin"}, relPaths(got))
	assert.Equal(t, []string{"data.lnk"}, seen)
}

func TestWalk_NormalizesPaths(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "café/agent.py", "x") // NFC; macOS may store NFD on disk

	got, err := Walk(root, nil, nil)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "café/agent.py", got[0].RelPath)
}
