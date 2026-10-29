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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// symlinkOrSkip creates link pointing at target, skipping the test where the
// OS refuses: Windows allows symlinks only with Developer Mode or elevation.
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()

	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}
}

// walkLinks walks root and returns its entries' rel paths and the links it
// met, by rel path.
func walkLinks(t *testing.T, root string, ignore IgnoreFunc) ([]string, map[string]Link) {
	t.Helper()

	links := map[string]Link{}

	got, err := Walk(root, ignore, func(l Link) {
		_, seen := links[l.RelPath]
		assert.False(t, seen, "%s reported twice", l.RelPath)

		links[l.RelPath] = l
	})
	require.NoError(t, err)

	return relPaths(got), links
}

func TestWalk_FollowsADirectoryLinkInsideTheProject(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "realdir/inner.py", "x")
	writeFile(t, root, "realdir/sub/deep.py", "y")
	symlinkOrSkip(t, filepath.Join(root, "realdir"), filepath.Join(root, "link_to_dir"))

	paths, links := walkLinks(t, root, nil)

	assert.ElementsMatch(t, []string{
		"realdir/inner.py", "realdir/sub/deep.py",
		"link_to_dir/inner.py", "link_to_dir/sub/deep.py",
	}, paths)
	assert.Equal(t, Link{
		RelPath: "link_to_dir", Target: filepath.Join(root, "realdir"), IsDir: true, Outcome: LinkFollowed,
	}, links["link_to_dir"])
}

// The entry reads what the link points at, so hashing it hashes the target.
func TestWalk_FollowsAFileLinkWithARelativeTarget(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "shared/config.py", "x")
	writeFile(t, root, "app/main.py", "y")
	symlinkOrSkip(t, filepath.Join("..", "shared", "config.py"), filepath.Join(root, "app", "config.py"))

	got, err := Walk(root, nil, nil)
	require.NoError(t, err)

	byRel := map[string]string{}
	for _, e := range got {
		byRel[e.RelPath] = e.AbsPath
	}

	require.Contains(t, byRel, "app/config.py")

	data, err := os.ReadFile(byRel["app/config.py"])
	require.NoError(t, err)
	assert.Equal(t, "x", string(data))
}

func TestWalk_FollowsAChainOfLinks(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "realfile.py", "x")
	symlinkOrSkip(t, filepath.Join(root, "realfile.py"), filepath.Join(root, "inner.lnk"))
	symlinkOrSkip(t, filepath.Join(root, "inner.lnk"), filepath.Join(root, "outer.lnk"))

	paths, links := walkLinks(t, root, nil)

	assert.ElementsMatch(t, []string{"realfile.py", "inner.lnk", "outer.lnk"}, paths)
	assert.Equal(t, LinkFollowed, links["outer.lnk"].Outcome)
	assert.Equal(t, LinkFollowed, links["inner.lnk"].Outcome)
}

func TestWalk_DoesNotFollowADanglingLink(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "agent.py", "x")
	symlinkOrSkip(t, filepath.Join(root, "nonexistent"), filepath.Join(root, "dangling.lnk"))

	paths, links := walkLinks(t, root, nil)

	assert.Equal(t, []string{"agent.py"}, paths)
	assert.Equal(t, Link{RelPath: "dangling.lnk", Outcome: LinkDangling}, links["dangling.lnk"])
}

// Nothing outside the project is read: neither the file nor the directory a
// link points at.
func TestWalk_DoesNotFollowALinkOutsideTheProject(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "agent.py", "x")

	external := t.TempDir()
	writeFile(t, external, "secret.txt", "secret")
	symlinkOrSkip(t, filepath.Join(external, "secret.txt"), filepath.Join(root, "secret.lnk"))
	symlinkOrSkip(t, external, filepath.Join(root, "external"))

	paths, links := walkLinks(t, root, nil)

	assert.Equal(t, []string{"agent.py"}, paths)
	assert.Equal(t, LinkOutside, links["secret.lnk"].Outcome)
	assert.False(t, links["secret.lnk"].IsDir)
	assert.Equal(t, LinkOutside, links["external"].Outcome)
	assert.True(t, links["external"].IsDir)
}

// A link out of the project and back in again stays inside it: what counts is
// where the chain ends.
func TestWalk_JudgesALinkByWhereItsChainEnds(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "realdir/inner.py", "x")

	outside := t.TempDir()
	symlinkOrSkip(t, filepath.Join(root, "realdir"), filepath.Join(outside, "back"))
	symlinkOrSkip(t, filepath.Join(outside, "back"), filepath.Join(root, "via_outside"))

	paths, links := walkLinks(t, root, nil)

	assert.Contains(t, paths, "via_outside/inner.py")
	assert.Equal(t, LinkFollowed, links["via_outside"].Outcome)
}

func TestWalk_DoesNotFollowALinkIntoADirectoryItIsInside(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a/file.py", "x")
	symlinkOrSkip(t, root, filepath.Join(root, "a", "to_root"))
	symlinkOrSkip(t, filepath.Join(root, "a"), filepath.Join(root, "a", "self"))

	paths, links := walkLinks(t, root, nil)

	assert.Equal(t, []string{"a/file.py"}, paths)
	assert.Equal(t, LinkLoop, links["a/to_root"].Outcome)
	assert.Equal(t, LinkLoop, links["a/self"].Outcome)
}

// Two directories linking to each other end the walk instead of recursing
// forever: each is followed once, from the other.
func TestWalk_StopsAtLinksThatPointAtEachOther(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a/a.py", "x")
	writeFile(t, root, "b/b.py", "y")
	symlinkOrSkip(t, filepath.Join(root, "b"), filepath.Join(root, "a", "to_b"))
	symlinkOrSkip(t, filepath.Join(root, "a"), filepath.Join(root, "b", "to_a"))

	paths, links := walkLinks(t, root, nil)

	assert.ElementsMatch(t, []string{
		"a/a.py", "a/to_b/b.py",
		"b/b.py", "b/to_a/a.py",
	}, paths)
	assert.Equal(t, LinkFollowed, links["a/to_b"].Outcome)
	assert.Equal(t, LinkLoop, links["a/to_b/to_a"].Outcome)
}

func TestWalk_IgnoreRulesApplyToLinksAndWhatTheyPointAt(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "app.py", "x")
	writeFile(t, root, "node_modules/pkg/index.js", "y")
	writeFile(t, root, "realdir/inner.py", "z")
	symlinkOrSkip(t, filepath.Join(root, "node_modules", "pkg"), filepath.Join(root, "vendor"))
	symlinkOrSkip(t, filepath.Join(root, "realdir"), filepath.Join(root, "ignored_link"))
	symlinkOrSkip(t, filepath.Join(root, "nonexistent"), filepath.Join(root, "ignored_dangling"))

	ignore := func(rel string, isDir bool) bool {
		return (rel == "node_modules" && isDir) || rel == "ignored_link" || (rel == "ignored_dangling" && isDir)
	}

	paths, links := walkLinks(t, root, ignore)

	assert.ElementsMatch(t, []string{"app.py", "realdir/inner.py"}, paths)
	assert.Equal(t, map[string]Link{
		"vendor": {
			RelPath: "vendor", Target: filepath.Join(root, "node_modules", "pkg"), IsDir: true, Outcome: LinkTargetIgnored,
		},
	}, links, "an ignored link is not reported, a dangling one under either spelling")
}
