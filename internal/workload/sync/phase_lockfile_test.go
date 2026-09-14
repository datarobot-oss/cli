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
	"github.com/datarobot/cli/internal/workload/ignore"
)

// noLockfileRunner is for tests whose project has no pyproject.toml, where the
// lockfile phase returns before the runner would be called. Naming it says
// "this test is not about uv" at the call site.
func noLockfileRunner(string) error { return nil }

// lockfileCurrent is the checker for tests that are not about staleness.
// Every engine here injects one: left nil the engine falls back to the
// production checker, which shells out to the real uv.
func lockfileCurrent(string) (bool, error) { return true, nil }

// lockfileStale answers "the lock no longer matches pyproject.toml".
func lockfileStale(string) (bool, error) { return false, nil }

// lockfileEngine builds an engine over dir with an injected LockfileRunner
// and an empty draft artifact (first-sync shape). The lockfile is reported
// as current, so the runner is reached only by the missing-lockfile path.
func lockfileEngine(t *testing.T, dir string, runner LockfileRunner) *Engine {
	t.Helper()

	return lockfileEngineWithCheck(t, dir, runner, lockfileCurrent)
}

// lockfileEngineWithCheck is lockfileEngine with the staleness check
// injected too, for the tests that are about it.
func lockfileEngineWithCheck(t *testing.T, dir string, runner LockfileRunner, check LockfileChecker) *Engine {
	t.Helper()

	e, err := newWithDeps(dir, Options{}, Deps{
		Files: &fakeFilesClient{},
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, "", ""), nil
			},
		},
		Now:           time.Now,
		Lockfile:      runner,
		LockfileCheck: check,
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
		[]string{".drignore", "pyproject.toml", "app.py", "uv.lock"},
		uploadPathsOf(plan))
	assert.True(t, e.lockfileGenerated)
	assert.Empty(t, e.lockfileHint)
}

func TestEngine_Plan_LeavesCurrentLockfileAlone(t *testing.T) {
	dir := initProject(t, map[string]string{
		"pyproject.toml": "[project]\nname = \"x\"\n",
		"uv.lock":        "version = 1\n",
	})

	runnerCalled := false
	runner := func(string) error {
		runnerCalled = true

		return nil
	}

	checked := false
	check := func(string) (bool, error) {
		checked = true

		return true, nil
	}

	e := lockfileEngineWithCheck(t, dir, runner, check)

	_, err := e.Plan()
	require.NoError(t, err)

	assert.True(t, checked, "an existing uv.lock is checked against pyproject.toml")
	assert.False(t, runnerCalled, "a current uv.lock is not re-locked")
	assert.False(t, e.lockfileGenerated)
	assert.Empty(t, e.lockfileHint)
}

// A stale uv.lock is the case nothing downstream catches: the build
// installs the lock as-is, so an edited pyproject.toml would produce an
// image missing the dependency that was added, with every step green.
func TestEngine_Plan_RefreshesStaleLockfile(t *testing.T) {
	dir := initProject(t, map[string]string{
		"pyproject.toml": "[project]\nname = \"x\"\n",
		"uv.lock":        "version = 1\n",
	})

	relocked := false
	runner := func(d string) error {
		relocked = true

		return os.WriteFile(filepath.Join(d, "uv.lock"), []byte("version = 2\n"), 0o600)
	}

	// Stale until the runner has rewritten it, current afterwards.
	check := func(string) (bool, error) { return relocked, nil }

	e := lockfileEngineWithCheck(t, dir, runner, check)

	plan, err := e.Plan()
	require.NoError(t, err)

	assert.True(t, relocked, "a stale uv.lock is re-locked")
	assert.True(t, e.lockfileGenerated)
	assert.Contains(t, uploadPathsOf(plan), "uv.lock", "the refreshed lock is uploaded with the rest")
}

// Refusing is the whole point: continuing here would upload the stale
// lock and build the wrong image, which is what the generate path can
// afford to do only because a missing lockfile fails the build loudly.
func TestEngine_Plan_StaleLockfileThatCannotBeRegeneratedRefuses(t *testing.T) {
	dir := initProject(t, map[string]string{
		"pyproject.toml": "[project]\nname = \"x\"\n",
		"uv.lock":        "version = 1\n",
	})

	runner := func(string) error { return errors.New("no solution found for left-pad") }

	e := lockfileEngineWithCheck(t, dir, runner, lockfileStale)

	_, err := e.Plan()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of date")
	assert.Contains(t, err.Error(), "nothing was uploaded")
	assert.Contains(t, err.Error(), "left-pad", "uv's own reason is carried through")
}

// `uv lock` in a workspace member exits 0 while writing the lockfile at
// the workspace root, leaving this directory exactly as stale as it was.
func TestEngine_Plan_StaleLockfileStillStaleAfterRelockRefuses(t *testing.T) {
	dir := initProject(t, map[string]string{
		"pyproject.toml": "[project]\nname = \"x\"\n",
		"uv.lock":        "version = 1\n",
	})

	e := lockfileEngineWithCheck(t, dir, func(string) error { return nil }, lockfileStale)

	_, err := e.Plan()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "still out of date")
	assert.Contains(t, err.Error(), "workspace")
}

// Not knowing is not the same as knowing it is wrong. A project whose
// lockfile is current synced with no uv on PATH before this check
// existed, so an unanswerable check warns and lets the sync through.
func TestEngine_Plan_UvMissingCannotCheckStalenessButDoesNotBlock(t *testing.T) {
	dir := initProject(t, map[string]string{
		"pyproject.toml": "[project]\nname = \"x\"\n",
		"uv.lock":        "version = 1\n",
	})

	runnerCalled := false
	runner := func(string) error {
		runnerCalled = true

		return nil
	}

	check := func(string) (bool, error) { return false, errUvNotFound }

	e := lockfileEngineWithCheck(t, dir, runner, check)

	_, err := e.Plan()
	require.NoError(t, err)

	assert.False(t, runnerCalled, "nothing to re-lock when staleness is unknown")
	assert.False(t, e.lockfileGenerated)
	assert.Contains(t, e.lockfileHint, "uv is not installed")
	assert.Contains(t, e.lockfileHint, "old lock")
}

func TestEngine_Plan_StalenessCheckFailureDoesNotBlock(t *testing.T) {
	dir := initProject(t, map[string]string{
		"pyproject.toml": "[project]\nname = \"x\"\n",
		"uv.lock":        "version = 1\n",
	})

	check := func(string) (bool, error) { return false, errors.New("uv lock --check failed: broken toml") }

	e := lockfileEngineWithCheck(t, dir, noLockfileRunner, check)

	_, err := e.Plan()
	require.NoError(t, err)

	assert.Contains(t, e.lockfileHint, "Could not check uv.lock")
	assert.Contains(t, e.lockfileHint, "broken toml")
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

func TestEngine_Plan_WarnsWhenIgnoreFileExcludesLockfile(t *testing.T) {
	dir := initProject(t, map[string]string{
		"pyproject.toml": "[project]\nname = \"x\"\n",
		"uv.lock":        "version = 1\n",
		ignore.FileName:  "uv.lock\n",
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
	assert.Contains(t, e.lockfileHint, "excluded by "+ignore.FileName)
}

// The hint tells the user to go edit a pattern, so it has to name the file
// they actually have. A project on the legacy name would otherwise be sent to
// a .drignore that does not exist.
func TestEngine_Plan_LockfileHintNamesTheLegacyIgnoreFile(t *testing.T) {
	dir := initProject(t, map[string]string{
		"pyproject.toml":      "[project]\nname = \"x\"\n",
		"uv.lock":             "version = 1\n",
		ignore.LegacyFileName: "uv.lock\n",
	})

	// initProject links the project, which drops the current-name template.
	// Remove it so the legacy file is the only one, as it would be in a
	// project set up before the rename.
	require.NoError(t, os.Remove(filepath.Join(dir, ignore.FileName)))

	e := lockfileEngine(t, dir, func(string) error { return nil })

	plan, err := e.Plan()
	require.NoError(t, err)

	assert.NotContains(t, uploadPathsOf(plan), "uv.lock")
	assert.Contains(t, e.lockfileHint, "excluded by "+ignore.LegacyFileName)
	assert.NotContains(t, e.lockfileHint, ignore.FileName)
}
