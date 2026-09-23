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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCmd_RequiresSpecFile(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec-file")
}

func TestCmd_SpecFileNotFound(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"--spec-file", "/nonexistent/workload.json"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file not found")
}

func TestCmd_InvalidSpecFailsBeforeNetwork(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"name": "wl"}`), 0o600))

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"--spec-file", path})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of 'artifactId'")
}

func TestCmd_InvalidOutputFormat(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"--spec-file", "x.json", "--output-format", "yaml"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid output format "yaml"`)
}

func TestCmd_RejectsPositionalArgs(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"unexpected", "--spec-file", "x.json"})

	err := cmd.Execute()
	require.Error(t, err)
}

// TestCmd_EnclavePinReachesTheWire asserts the pinned bytes are what gets
// POSTed: dropping the ApplyEnclavePin result on the floor would create an
// unpinned workload and exit 0, which no other test can catch.
func TestCmd_EnclavePinReachesTheWire(t *testing.T) {
	var posted []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posted, _ = io.ReadAll(r.Body)

		fmt.Fprint(w, `{"id":"wl-1","name":"my-app","status":"submitted"}`)
	}))

	defer srv.Close()

	viperx.Set(config.DataRobotURL, srv.URL)
	viperx.Set(config.DataRobotAPIKey, "test-token")
	viperx.Set(config.SkipAuthKey, true)

	t.Cleanup(viperx.Reset)

	dir := t.TempDir()
	path := filepath.Join(dir, "spec.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"name": "wl", "artifactId": "art-1"}`), 0o600))

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"--spec-file", path, "--enclave", "prod-east", "--use-case-id", "68b0aa11bb22cc33dd44ee55"})

	require.NoError(t, cmd.Execute())

	var body map[string]any

	require.NoError(t, json.Unmarshal(posted, &body), "the body must be JSON, not a base64-encoded string")

	runtime, ok := body["runtime"].(map[string]any)

	require.True(t, ok)
	assert.Equal(t, "manual", runtime["enclaveSelectionPolicy"])
	assert.Equal(t, []any{"prod-east"}, runtime["enclaves"])
	assert.Equal(t, "68b0aa11bb22cc33dd44ee55", body["useCaseId"])
}

func TestCmd_BlankEnclaveFailsBeforeNetwork(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"name": "wl", "artifactId": "art-1"}`), 0o600))

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"--spec-file", path, "--enclave", "", "--use-case-id", "68b0aa11bb22cc33dd44ee55"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --enclave")
}

func TestCmd_EnclaveConflictsWithSpecPin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.json")
	spec := `{"name": "wl", "artifactId": "art-1", "runtime": {"enclaves": ["prod-west"], "enclaveSelectionPolicy": "manual"}}`
	require.NoError(t, os.WriteFile(path, []byte(spec), 0o600))

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"--spec-file", path, "--enclave", "prod-east", "--use-case-id", "68b0aa11bb22cc33dd44ee55"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already sets")
}

func TestCmd_UseCaseAloneReachesTheWireWithAvailability(t *testing.T) {
	var posted []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posted, _ = io.ReadAll(r.Body)

		fmt.Fprint(w, `{"id":"wl-1","name":"my-app","status":"submitted"}`)
	}))

	defer srv.Close()

	viperx.Set(config.DataRobotURL, srv.URL)
	viperx.Set(config.DataRobotAPIKey, "test-token")
	viperx.Set(config.SkipAuthKey, true)

	t.Cleanup(viperx.Reset)

	dir := t.TempDir()
	path := filepath.Join(dir, "spec.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"name": "wl", "artifactId": "art-1"}`), 0o600))

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"--spec-file", path, "--use-case-id", "68b0aa11bb22cc33dd44ee55"})

	require.NoError(t, cmd.Execute())

	var body map[string]any

	require.NoError(t, json.Unmarshal(posted, &body))
	assert.Equal(t, "68b0aa11bb22cc33dd44ee55", body["useCaseId"])

	runtime, ok := body["runtime"].(map[string]any)

	require.True(t, ok)
	assert.Equal(t, "availability", runtime["enclaveSelectionPolicy"])
}

func TestCmd_EnclaveWithoutUseCaseFailsBeforeNetwork(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"name": "wl", "artifactId": "art-1"}`), 0o600))

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"--spec-file", path, "--enclave", "prod-east"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--enclave requires --use-case-id")
}

func TestCmd_UseCaseConflictsWithSpecUseCase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.json")
	spec := `{"name": "wl", "artifactId": "art-1", "useCaseId": "68b0ffffffffffffffffffff"}`
	require.NoError(t, os.WriteFile(path, []byte(spec), 0o600))

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"--spec-file", path, "--use-case-id", "68b0aa11bb22cc33dd44ee55"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already sets useCaseId")
}

// A spec that already names a Use Case can be pinned with --enclave alone.
func TestCmd_EnclaveWithSpecUseCaseReachesTheWire(t *testing.T) {
	var posted []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posted, _ = io.ReadAll(r.Body)

		fmt.Fprint(w, `{"id":"wl-1","name":"my-app","status":"submitted"}`)
	}))

	defer srv.Close()

	viperx.Set(config.DataRobotURL, srv.URL)
	viperx.Set(config.DataRobotAPIKey, "test-token")
	viperx.Set(config.SkipAuthKey, true)

	t.Cleanup(viperx.Reset)

	dir := t.TempDir()
	path := filepath.Join(dir, "spec.json")
	spec := `{"name": "wl", "artifactId": "art-1", "useCaseId": "68b0aa11bb22cc33dd44ee55"}`
	require.NoError(t, os.WriteFile(path, []byte(spec), 0o600))

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"--spec-file", path, "--enclave", "prod-east"})

	require.NoError(t, cmd.Execute())

	var body map[string]any

	require.NoError(t, json.Unmarshal(posted, &body))
	assert.Equal(t, "68b0aa11bb22cc33dd44ee55", body["useCaseId"])

	runtime, ok := body["runtime"].(map[string]any)

	require.True(t, ok)
	assert.Equal(t, "manual", runtime["enclaveSelectionPolicy"])
	assert.Equal(t, []any{"prod-east"}, runtime["enclaves"])
}

func TestCmd_MalformedUseCaseIDFailsBeforeNetwork(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"name": "wl", "artifactId": "art-1"}`), 0o600))

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"--spec-file", path, "--use-case-id", "not-an-id"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --use-case-id")
}

func TestPlacementMode(t *testing.T) {
	for name, c := range map[string]struct {
		args []string
		want string
	}{
		"no placement flags": {nil, "none"},
		"use case only":      {[]string{"--use-case-id", "68b0aa11bb22cc33dd44ee55"}, "availability"},
		"enclave":            {[]string{"--enclave", "prod-east", "--use-case-id", "68b0aa11bb22cc33dd44ee55"}, "manual"},
	} {
		t.Run(name, func(t *testing.T) {
			cmd := Cmd()
			require.NoError(t, cmd.ParseFlags(c.args))
			assert.Equal(t, c.want, placementMode(cmd))
		})
	}
}
