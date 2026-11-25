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

package sync

import (
	"os"
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

// A whole sync over junctions: one inside the project is uploaded through,
// and its local copy wins over a server edit; one out of the project is
// neither uploaded nor written through by a server file at its path.
func TestSync_Junctions(t *testing.T) {
	const (
		catalogID = "cat-junctions"
		versionID = "ver-junctions"
		// A teammate synced since: the artifact points at a newer version.
		newerID = "ver-junctions-2"
	)

	dir := syncedProject(t, map[string]string{
		"app.py":         "print('hi')\n",
		"shared/util.py": "x = 1\n",
		"lib/util.py":    "x = 1\n",
	}, catalogID, versionID)

	require.NoError(t, os.RemoveAll(filepath.Join(dir, "lib")))
	junction(t, filepath.Join(dir, "shared"), filepath.Join(dir, "lib"))

	external := outsideDir(t, "ext", "kept.py")
	junction(t, external, filepath.Join(dir, "ext"))

	seed := seededServerContents(t, dir, map[string]string{"app.py": "", "shared/util.py": ""}, map[string][]byte{
		"lib/util.py": []byte("x = 2  # edited on the server\n"),
		"ext/x.py":    []byte("from the server\n"),
	})
	fake := (&fakeFilesClient{stageID: "stage-junctions", versionID: "ver-junctions-3"}).
		withVersionContent(catalogID, newerID, seed)

	e := engineFor(t, dir, Options{Yes: true}, fake, catalogID, newerID)

	plan, err := e.Plan()
	require.NoError(t, err)

	assert.Equal(t, []string{"lib"}, e.linkedPaths)
	assert.Equal(t, []SkippedSymlink{{Path: "ext", IsDir: true, Reason: "outside_project"}}, e.skippedSymlinks)

	assert.Empty(t, plan.Downloads)
	assert.Empty(t, plan.Conflicts)
	assert.Equal(t, []string{"lib/util.py"}, uploadPathsOf(plan))
	assert.Equal(t, []string{"lib/util.py"}, plan.ReplacedRemote)

	skipped := make([]string, 0, len(plan.Skipped))
	for _, fa := range plan.Skipped {
		skipped = append(skipped, fa.Path)
	}

	assert.Equal(t, []string{"ext/x.py"}, skipped)

	_, err = e.Execute(plan)
	require.NoError(t, err)

	assert.Equal(t, []byte("x = 1\n"), fake.uploadedFiles["lib/util.py"], "the upload reads through the junction")

	data, err := os.ReadFile(filepath.Join(dir, "shared", "util.py"))
	require.NoError(t, err)
	assert.Equal(t, "x = 1\n", string(data), "nothing is written through the junction")
	assert.NoFileExists(t, filepath.Join(external, "x.py"), "nothing is written out of the project")
}
