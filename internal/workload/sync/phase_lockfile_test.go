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
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/datarobot/cli/internal/workload"
)

// lockfileEngine builds an engine over dir with an injected LockfileRunner
// and an empty draft artifact (first-sync shape).
func lockfileEngine(t *testing.T, dir string, runner LockfileRunner) *Engine {
	t.Helper()

	e, err := newWithDeps(dir, Options{}, Deps{
		Files: &fakeFilesClient{},
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, "", ""), nil
			},
		},
		Now:      time.Now,
		Lockfile: runner,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Close() })

	return e
}

func uploadPathsOf(plan *SyncPlan) []string {
	paths := make([]string, 0, len(plan.Uploads))
	for _, fa := range plan.Uploads {
		paths = append(paths, fa.Path)
	}

	return paths
}

func TestEngine_Plan_GeneratesLockfileWhenMissing(t *testing.T) {
	dir := initProject(t, map[string]string{
		"pyproject.toml": "[project]\nname = \"x\"\n",
		"app.py":         "print('hi')\n",
	})

	runner := func(projectDir string) error {
		return os.WriteFile(filepath.Join(projectDir, uvLockFile), []byte("version = 1\n"), 0o644)
	}

	e := lockfileEngine(t, dir, runner)

	plan, err := e.Plan()
	require.NoError(t, err)

	// The generated lock exists before the walk, so it flows through the
	// normal diff/upload pipeline like any user file.
	assert.ElementsMatch(t,
		[]string{".wapiignore", "pyproject.toml", "app.py", "uv.lock"},
		uploadPathsOf(plan))
	assert.True(t, e.lockfileGenerated)
	assert.Empty(t, e.lockfileHint)
}

func TestEngine_Plan_LeavesExistingLockfileAlone(t *testing.T) {
	dir := initProject(t, map[string]string{
		"pyproject.toml": "[project]\nname = \"x\"\n",
		"uv.lock":        "version = 1\n",
	})

	runnerCalled := false
	runner := func(string) error {
		runnerCalled = true

		return nil
	}

	e := lockfileEngine(t, dir, runner)

	_, err := e.Plan()
	require.NoError(t, err)

	assert.False(t, runnerCalled, "runner must not run when uv.lock already exists")
	assert.False(t, e.lockfileGenerated)
	assert.Empty(t, e.lockfileHint)
}

func TestEngine_Plan_SkipsLockfileWithoutPyproject(t *testing.T) {
	dir := initProject(t, map[string]string{"agent.py": "x"})

	runnerCalled := false
	runner := func(string) error {
		runnerCalled = true

		return nil
	}

	e := lockfileEngine(t, dir, runner)

	_, err := e.Plan()
	require.NoError(t, err)

	assert.False(t, runnerCalled, "runner must not run for non-Python projects")
	assert.Empty(t, e.lockfileHint)
}

func TestEngine_Plan_UvMissingHintsWithoutBlocking(t *testing.T) {
	dir := initProject(t, map[string]string{
		"pyproject.toml": "[project]\nname = \"x\"\n",
	})

	e := lockfileEngine(t, dir, func(string) error { return errUvNotFound })

	plan, err := e.Plan()
	require.NoError(t, err, "missing uv must not fail the sync")

	assert.NotContains(t, uploadPathsOf(plan), "uv.lock")
	assert.False(t, e.lockfileGenerated)
	assert.Contains(t, e.lockfileHint, "uv is not installed")
	assert.Contains(t, e.lockfileHint, "uv lock")
}

func TestEngine_Plan_LockGenerationFailureHintsWithoutBlocking(t *testing.T) {
	dir := initProject(t, map[string]string{
		"pyproject.toml": "[project]\nname = \"x\"\n",
	})

	e := lockfileEngine(t, dir, func(string) error {
		return errors.New("uv lock failed: no wheels for left-pad")
	})

	plan, err := e.Plan()
	require.NoError(t, err, "lock-generation failure must not fail the sync")

	assert.NotContains(t, uploadPathsOf(plan), "uv.lock")
	assert.Contains(t, e.lockfileHint, "Could not generate uv.lock")
	assert.Contains(t, e.lockfileHint, "left-pad")
}

func TestEngine_Plan_RunnerSucceedsButLockfileAbsentHints(t *testing.T) {
	// uv workspace member: `uv lock` exits 0 but writes the lockfile at the
	// workspace root, so projectDir gains no uv.lock. Must not claim success.
	dir := initProject(t, map[string]string{
		"pyproject.toml": "[project]\nname = \"x\"\n",
	})

	e := lockfileEngine(t, dir, func(string) error { return nil }) // exits 0, writes nothing

	plan, err := e.Plan()
	require.NoError(t, err)

	assert.NotContains(t, uploadPathsOf(plan), "uv.lock")
	assert.False(t, e.lockfileGenerated, "must not report success when no lockfile appeared")
	assert.Contains(t, e.lockfileHint, "did not create uv.lock")
	assert.Contains(t, e.lockfileHint, "workspace")
}

func TestEngine_Plan_WarnsWhenWapiignoreExcludesLockfile(t *testing.T) {
	dir := initProject(t, map[string]string{
		"pyproject.toml": "[project]\nname = \"x\"\n",
		"uv.lock":        "version = 1\n",
		".wapiignore":    "uv.lock\n",
	})

	runnerCalled := false
	e := lockfileEngine(t, dir, func(string) error {
		runnerCalled = true

		return nil
	})

	plan, err := e.Plan()
	require.NoError(t, err)

	assert.False(t, runnerCalled, "lock exists; generation must not run")
	assert.NotContains(t, uploadPathsOf(plan), "uv.lock", "ignored lock must not upload")
	assert.Contains(t, e.lockfileHint, "excluded by .wapiignore")
}
