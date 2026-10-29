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

package create

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/cmd/job/internal/jobtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func run(t *testing.T, args ...string) error {
	t.Helper()

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs(args)

	return cmd.Execute()
}

func TestCreate_NeedsANameAndAnArtifactBeforeAnyRequest(t *testing.T) {
	err := run(t, "--artifact-id", "art-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "needs a name")

	err = run(t, "--name", "train")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "needs a job artifact")
}

func TestCreate_PostsTheFlagsAndTheRuntimeFile(t *testing.T) {
	runtime := filepath.Join(t.TempDir(), "runtime.yaml")
	require.NoError(t, os.WriteFile(runtime,
		[]byte("containerGroups:\n  - name: default\n    resourceBundles: [gpu.a100.1]\nenclaveSelectionPolicy: availability\n"), 0o600))

	jobtest.Serve(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v2/jobs/", r.URL.Path)

		sent := jobtest.Body(t, r)
		assert.Equal(t, "train", sent["name"])
		assert.Equal(t, "art-1", sent["artifactId"])
		assert.Equal(t, "uc-1", sent["useCaseId"])
		assert.Equal(t, "availability", sent["runtime"].(map[string]any)["enclaveSelectionPolicy"])

		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, jobtest.JobDoc)
	})

	err := run(t, "--name", "train", "--artifact-id", "art-1", "--use-case-id", "uc-1", "--runtime-file", runtime)

	require.NoError(t, err)
}

func TestCreate_TheRuntimeIsSetInOnePlace(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "job.yaml")
	runtime := filepath.Join(dir, "runtime.yaml")

	require.NoError(t, os.WriteFile(spec, []byte("name: train\nartifactId: art-1\nruntime: {}\n"), 0o600))
	require.NoError(t, os.WriteFile(runtime, []byte("containerGroups: []\n"), 0o600))

	err := run(t, "--spec-file", spec, "--runtime-file", runtime)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "--runtime-file contradicts runtime")
}

func TestCreate_ShowsTheServersReason(t *testing.T) {
	jobtest.Serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"detail": "Artifact art-1 is a 'service' artifact; a job runs only 'job' artifacts."}`)
	})

	err := run(t, "--name", "train", "--artifact-id", "art-1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "a job runs only 'job' artifacts")
}
