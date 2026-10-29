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

//go:build windows

package fileops

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// junction links link to the directory target the way `mklink /J` does,
// which, unlike a symlink, needs no special rights.
func junction(t *testing.T, target, link string) {
	t.Helper()

	out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	require.NoError(t, err, string(out))
}

// Go reports a junction as an irregular file, not a directory or a symlink,
// and the walk used to drop it without a word.
func TestWalk_FollowsAJunctionInsideTheProject(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "realdir/inner.py", "x")
	junction(t, filepath.Join(root, "realdir"), filepath.Join(root, "linked"))

	paths, links := walkLinks(t, root, nil)

	assert.ElementsMatch(t, []string{"realdir/inner.py", "linked/inner.py"}, paths)
	assert.Equal(t, LinkFollowed, links["linked"].Outcome)
	assert.True(t, links["linked"].IsDir)
}

func TestWalk_DoesNotFollowAJunctionOutsideTheProject(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "agent.py", "x")

	external := t.TempDir()
	writeFile(t, external, "secret.txt", "secret")
	junction(t, external, filepath.Join(root, "external"))

	paths, links := walkLinks(t, root, nil)

	assert.Equal(t, []string{"agent.py"}, paths)
	assert.Equal(t, LinkOutside, links["external"].Outcome)
}

func TestWalk_DoesNotFollowAJunctionBackToTheRoot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a/file.py", "x")
	junction(t, root, filepath.Join(root, "a", "to_root"))

	paths, links := walkLinks(t, root, nil)

	assert.Equal(t, []string{"a/file.py"}, paths)
	assert.Equal(t, LinkLoop, links["a/to_root"].Outcome)
}
