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

package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()

	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}
}

// A server file at the path of a link the walk skipped is left alone: writing
// it would go through the link, here out of the project.
func TestSync_PullsNothingThroughALinkItDidNotFollow(t *testing.T) {
	const (
		catalogID = "cat-unfollowed"
		versionID = "ver-unfollowed"
	)

	dir := syncedProject(t, map[string]string{"app.py": "print('hi')\n"}, catalogID, versionID)

	secret := outsideFile(t, "secret.txt")
	external := outsideDir(t, "ext", "kept.py")

	symlinkOrSkip(t, secret, filepath.Join(dir, "secret.txt"))
	symlinkOrSkip(t, external, filepath.Join(dir, "ext"))

	seed := seededServerContents(t, dir, map[string]string{"app.py": ""}, map[string][]byte{
		"secret.txt": []byte("from the server\n"),
		"ext/x.py":   []byte("from the server\n"),
	})
	fake := (&fakeFilesClient{}).withVersionContent(catalogID, versionID, seed)

	e := engineFor(t, dir, Options{Verify: true, Yes: true}, fake, catalogID, versionID)

	plan, err := e.Plan()
	require.NoError(t, err)

	assert.Empty(t, plan.Downloads)

	skipped := make([]string, 0, len(plan.Skipped))
	for _, fa := range plan.Skipped {
		skipped = append(skipped, fa.Path)
	}

	assert.ElementsMatch(t, []string{"secret.txt", "ext/x.py"}, skipped)

	_, err = e.Execute(plan)
	require.NoError(t, err)

	data, err := os.ReadFile(secret)
	require.NoError(t, err)
	assert.Equal(t, "outside content\n", string(data), "the file outside the project is untouched")
	assert.NoFileExists(t, filepath.Join(external, "x.py"), "nothing is written into the directory outside the project")
}

// A teammate with the same link deleted a file the link reaches: removing it
// here removes the linked copy too, so the sync uploads nothing for it.
func TestSync_ALinkedFileDeletedOnTheServerGoesInBothPlaces(t *testing.T) {
	const (
		catalogID = "cat-linked-delete"
		versionID = "ver-linked-delete"
		newerID   = "ver-linked-delete-2"
	)

	dir := syncedProject(t, map[string]string{
		"app.py":      "print('hi')\n",
		"shared/x.py": "x = 1\n",
		"lib/x.py":    "x = 1\n",
	}, catalogID, versionID)

	require.NoError(t, os.RemoveAll(filepath.Join(dir, "lib")))
	symlinkOrSkip(t, "shared", filepath.Join(dir, "lib"))

	seed := seededServerContents(t, dir, map[string]string{"app.py": ""}, nil)
	fake := (&fakeFilesClient{}).withVersionContent(catalogID, newerID, seed)

	e := engineFor(t, dir, Options{Yes: true}, fake, catalogID, newerID)

	plan, err := e.Plan()
	require.NoError(t, err)

	assert.Empty(t, plan.Uploads)
	assert.Empty(t, plan.ReplacedRemote)

	_, err = e.Execute(plan)
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(dir, "shared", "x.py"))
}

// The local copy of a linked path replaces a server change to it, and says so.
func TestSync_WarnsWhenALinkedPathReplacesAServerChange(t *testing.T) {
	const (
		catalogID = "cat-linked"
		versionID = "ver-linked"
		// A teammate synced since: the artifact points at a newer version.
		newerID = "ver-linked-2"
	)

	dir := syncedProject(t, map[string]string{
		"shared/x.py": "x = 1\n",
		"lib/x.py":    "x = 1\n",
	}, catalogID, versionID)

	require.NoError(t, os.RemoveAll(filepath.Join(dir, "lib")))
	symlinkOrSkip(t, "shared", filepath.Join(dir, "lib"))

	seed := seededServerContents(t, dir, map[string]string{"shared/x.py": ""}, map[string][]byte{
		"lib/x.py": []byte("x = 2  # edited on the server\n"),
	})
	fake := (&fakeFilesClient{}).withVersionContent(catalogID, newerID, seed)

	e := engineFor(t, dir, Options{DryRun: true, Yes: true}, fake, catalogID, newerID)

	logged := captureWarnLog(t, func() {
		plan, err := e.Plan()
		require.NoError(t, err)

		assert.Empty(t, plan.Downloads)
		assert.Contains(t, uploadPathsOf(plan), "lib/x.py")
	})

	assert.Contains(t, logged, "lib/x.py changed on the server, but it is reached through a symlink")
}
