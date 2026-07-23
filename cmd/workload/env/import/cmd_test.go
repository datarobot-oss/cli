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

package importcmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func installTestServer(t *testing.T, srv *httptest.Server) {
	t.Helper()

	viperx.Set("skip_auth", true)
	viperx.Set(config.DataRobotURL, srv.URL+"/api/v2")
	viperx.Set(config.DataRobotAPIKey, "test-token")

	t.Cleanup(func() {
		srv.Close()
		viperx.Reset()
	})
}

func writeTempEnvFile(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.env")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))

	return path
}

func TestCmd_RequiresWorkloadID(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.Error(t, err)
}

func TestLoadVarsFromFile_MissingFileErrors(t *testing.T) {
	_, err := loadVarsFromFile(filepath.Join(t.TempDir(), "does-not-exist.env"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read env file")
}

func TestLoadVarsFromFile_EmptyFileErrors(t *testing.T) {
	path := writeTempEnvFile(t, "\n# just a comment\n\n")

	_, err := loadVarsFromFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no environment variables found")
}

// TestLoadVarsFromFile_ParsesSortsAndRecognizesCredentialSyntax exercises
// standard dotenv syntax (comments, blank lines, quoting) and confirms the
// same NAME=dr-credential:<id>/<key> recognition `env set` uses applies
// here too, plus that output is sorted by name for deterministic PATCH
// bodies (map iteration order is otherwise randomized).
func TestLoadVarsFromFile_ParsesSortsAndRecognizesCredentialSyntax(t *testing.T) {
	path := writeTempEnvFile(t, `# a comment
LOG_LEVEL=debug

QUOTED="has spaces"
API_KEY=dr-credential:cred-1/apiToken
`)

	vars, err := loadVarsFromFile(path)
	require.NoError(t, err)
	require.Len(t, vars, 3)

	// Sorted by name: API_KEY, LOG_LEVEL, QUOTED.
	assert.Equal(t, "API_KEY", vars[0].Name)
	assert.Equal(t, workload.EnvironmentVarSourceDRCredential, vars[0].Source)
	assert.Equal(t, "cred-1", vars[0].DRCredentialID)
	assert.Equal(t, "apiToken", vars[0].Key)

	assert.Equal(t, "LOG_LEVEL", vars[1].Name)
	assert.Equal(t, "debug", vars[1].Value)

	assert.Equal(t, "QUOTED", vars[2].Name)
	assert.Equal(t, "has spaces", vars[2].Value)
}

func TestLoadVarsFromFile_InvalidNamePropagatesError(t *testing.T) {
	path := writeTempEnvFile(t, `1BAD=x`)

	_, err := loadVarsFromFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid environment variable name")
}

// TestCmd_DefaultsToDotEnvInCurrentDirectory guards the documented default:
// no --file means read ./.env.
func TestCmd_DefaultsToDotEnvInCurrentDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("FROM_DEFAULT=1\n"), 0o600))
	t.Chdir(dir)

	var patchedBody map[string]any

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/wl-1/replacement/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"detail":"There is no active replacement for this workload."}`)
	})
	mux.HandleFunc("/api/v2/workloads/wl-1/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":"wl-1","artifactId":"art-1"}`)
	})
	mux.HandleFunc("/api/v2/artifacts/art-1/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&patchedBody))
		}

		fmt.Fprint(w, `{
			"id": "art-1",
			"status": "draft",
			"spec": {"containerGroups": [{"containers": [{
				"name": "main", "primary": true,
				"environmentVars": []
			}]}]}
		}`)
	})

	installTestServer(t, httptest.NewServer(mux))

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"wl-1", "--stage"})

	err := cmd.Execute()
	require.NoError(t, err)
	require.NotNil(t, patchedBody, "the artifact must have been patched")

	spec := patchedBody["spec"].(map[string]any)
	containers := spec["containerGroups"].([]any)[0].(map[string]any)["containers"].([]any)
	envVars := containers[0].(map[string]any)["environmentVars"].([]any)
	require.Len(t, envVars, 1)
	assert.Equal(t, "FROM_DEFAULT", envVars[0].(map[string]any)["name"])
}

// TestCmd_FileValuesTakePrecedenceOverExisting is the merge-semantics
// requirement: a name present both in the file and already on the workload
// ends up with the file's value, an ordinary upsert-by-name.
func TestCmd_FileValuesTakePrecedenceOverExisting(t *testing.T) {
	path := writeTempEnvFile(t, "EXISTING=from-file\nNEW=added\n")

	var patchedBody map[string]any

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/wl-1/replacement/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"detail":"There is no active replacement for this workload."}`)
	})
	mux.HandleFunc("/api/v2/workloads/wl-1/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":"wl-1","artifactId":"art-1"}`)
	})
	mux.HandleFunc("/api/v2/artifacts/art-1/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&patchedBody))
		}

		fmt.Fprint(w, `{
			"id": "art-1",
			"status": "draft",
			"spec": {"containerGroups": [{"containers": [{
				"name": "main", "primary": true,
				"environmentVars": [{"name": "EXISTING", "value": "from-workload"}, {"name": "UNTOUCHED", "value": "stays"}]
			}]}]}
		}`)
	})

	installTestServer(t, httptest.NewServer(mux))

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"wl-1", "--file", path, "--stage"})

	err := cmd.Execute()
	require.NoError(t, err)
	require.NotNil(t, patchedBody)

	spec := patchedBody["spec"].(map[string]any)
	containers := spec["containerGroups"].([]any)[0].(map[string]any)["containers"].([]any)
	envVars := containers[0].(map[string]any)["environmentVars"].([]any)
	require.Len(t, envVars, 3, "EXISTING updated in place, UNTOUCHED preserved, NEW appended")

	byName := map[string]string{}

	for _, v := range envVars {
		m := v.(map[string]any)
		byName[m["name"].(string)] = m["value"].(string)
	}

	assert.Equal(t, "from-file", byName["EXISTING"], "the file's value must win over the workload's existing value")
	assert.Equal(t, "stays", byName["UNTOUCHED"])
	assert.Equal(t, "added", byName["NEW"])
}

// TestCmd_ActiveReplacementBlocksBeforeReadingFile guards the upfront
// fail-fast check applying here too: a bad --file path must not even
// matter if a replacement is already in flight.
func TestCmd_ActiveReplacementBlocksBeforeReadingFile(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/wl-1/replacement/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"candidateArtifactId":"art-9","status":"submitted"}`)
	})

	installTestServer(t, httptest.NewServer(mux))

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"wl-1", "--file", "/nonexistent/path/does/not/matter.env", "--yes"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already has a replacement in progress")
}

// TestCmd_StageSkipsActiveReplacementCheck guards the deliberate exception:
// --stage never touches the live rollout machinery, so it must proceed even
// while a replacement is in flight.
func TestCmd_StageSkipsActiveReplacementCheck(t *testing.T) {
	path := writeTempEnvFile(t, "LOG_LEVEL=debug\n")

	var replacementChecked bool

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/wl-1/replacement/", func(w http.ResponseWriter, _ *http.Request) {
		replacementChecked = true

		fmt.Fprint(w, `{"candidateArtifactId":"art-9","status":"submitted"}`)
	})
	mux.HandleFunc("/api/v2/workloads/wl-1/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":"wl-1","artifactId":"art-1"}`)
	})
	mux.HandleFunc("/api/v2/artifacts/art-1/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{
			"id": "art-1",
			"status": "draft",
			"spec": {"containerGroups": [{"containers": [{
				"name": "main", "primary": true,
				"environmentVars": []
			}]}]}
		}`)
	})

	installTestServer(t, httptest.NewServer(mux))

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"wl-1", "--file", path, "--stage"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.False(t, replacementChecked, "--stage must skip the in-flight-replacement check entirely")
}
