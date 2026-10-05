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

package up

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unboundGeneratedManifest is the Dockerfile fixture with the platform
// writing the Dockerfile from a base environment instead.
var unboundGeneratedManifest = strings.Replace(unboundDockerfileManifest,
	"                source: provided\n",
	"                source: generated\n"+
		"                executionEnvironmentId: 6890000000000000000000e1\n"+
		"                executionEnvironmentVersionId: 6890000000000000000000e2\n"+
		"                entrypoint: [\"node\", \"app.js\"]\n", 1)

var pythonEnvironment = workload.ExecutionEnvironment{
	ID: "6890000000000000000000e1", Name: "[DataRobot] Python 3.12 Drop-In", ProgrammingLanguage: "python",
}

// neverCreates fails the test if the run reaches the artifact create: the
// refusal has to come first, which is the whole point of it.
func neverCreates(t *testing.T) func(any) (*workload.Artifact, error) {
	return func(any) (*workload.Artifact, error) {
		t.Fatal("the run created an artifact the platform could not have built")

		return nil, nil
	}
}

func writeFiles(t *testing.T, dir string, names ...string) {
	t.Helper()

	for _, name := range names {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x\n"), 0o600))
	}
}

// A generated build the platform cannot make is refused before anything is
// created, with the same findings config gives: the missing file pair, or a
// base image of another language than the project. The build used to fail
// only after the artifact existed and the code was synced.
func TestRun_RefusesAGeneratedBuildThePlatformCannotMake(t *testing.T) {
	t.Run("no file pair the platform builds from", func(t *testing.T) {
		install(t, fakes{newArtifact: neverCreates(t), execEnv: func(string) (workload.ExecutionEnvironment, error) {
			return pythonEnvironment, nil
		}})

		_, _, err := runIn(t, unboundGeneratedManifest, Options{NonInteractive: true})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nothing was deployed")
		assert.Contains(t, err.Error(), "neither pyproject.toml with uv.lock nor package.json with package-lock.json")
		assert.Contains(t, err.Error(), "dr workload config")
	})

	for name, dryRun := range map[string]bool{"a deploy": false, "a dry run": true} {
		t.Run(name+" on a node project with a python environment", func(t *testing.T) {
			install(t, fakes{newArtifact: neverCreates(t), execEnv: func(string) (workload.ExecutionEnvironment, error) {
				return pythonEnvironment, nil
			}})

			dir := t.TempDir()
			writeManifest(t, dir, unboundGeneratedManifest)
			writeFiles(t, dir, "package.json", "package-lock.json")

			_, err := Run(t.Context(), Options{Dir: dir, NonInteractive: true, DryRun: dryRun, Stderr: &bytes.Buffer{}})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "Python 3.12 Drop-In is a python environment")
			assert.Contains(t, err.Error(), "node project (package.json, package-lock.json)")
		})
	}

	t.Run("a python project with a python environment goes through", func(t *testing.T) {
		install(t, fakes{execEnv: func(string) (workload.ExecutionEnvironment, error) { return pythonEnvironment, nil }})

		dir := t.TempDir()
		writeManifest(t, dir, unboundGeneratedManifest)
		writeFiles(t, dir, "pyproject.toml", "uv.lock")

		result, err := Run(t.Context(), Options{Dir: dir, NonInteractive: true, DryRun: true, Stderr: &bytes.Buffer{}})
		require.NoError(t, err)
		assert.Empty(t, result.Plan.Unbuildable)
	})

	t.Run("an environment that cannot be read stops the run", func(t *testing.T) {
		install(t, fakes{newArtifact: neverCreates(t), execEnv: func(id string) (workload.ExecutionEnvironment, error) {
			return workload.ExecutionEnvironment{}, assert.AnError
		}})

		dir := t.TempDir()
		writeManifest(t, dir, unboundGeneratedManifest)
		writeFiles(t, dir, "pyproject.toml", "uv.lock")

		_, err := Run(t.Context(), Options{Dir: dir, NonInteractive: true, Stderr: &bytes.Buffer{}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot read execution environment 6890000000000000000000e1")
	})
}

// A bound workload deploying from a directory with none of the usual project
// files pulls its code first, so the directory is not judged before the pull.
func TestUnbuildableGenerated_PullIntoAnEmptyDirectoryIsNotJudged(t *testing.T) {
	install(t, fakes{execEnv: func(string) (workload.ExecutionEnvironment, error) { return pythonEnvironment, nil }})
	force(t, &projectLinkedFn, func(string) bool { return false })

	dir := t.TempDir()
	writeManifest(t, dir, unboundGeneratedManifest)

	loaded, err := load(dir, Options{NonInteractive: true})
	require.NoError(t, err)

	live := Live{WorkloadID: "wl-1", ArtifactID: "art-1", State: StateRunning}
	plan := Plan{Code: CodeChange{Applies: true, FirstDeploy: true}}

	problem, err := unbuildableGenerated(loaded, live, plan)
	require.NoError(t, err)
	assert.Empty(t, problem)

	// With project files of its own, the directory is uploaded as it is, so
	// it is judged.
	writeFiles(t, dir, "package.json", "package-lock.json")

	problem, err = unbuildableGenerated(loaded, live, plan)
	require.NoError(t, err)
	assert.Contains(t, problem, "node project")
}
