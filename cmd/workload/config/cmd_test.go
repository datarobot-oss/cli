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

package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/wizard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runCmd executes the command with the auth check removed, capturing the two
// streams separately: keeping them apart is the point of several tests below.
func runCmd(t *testing.T, args ...string) (stdout, stderr *bytes.Buffer, err error) {
	t.Helper()

	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)

	return stdout, stderr, cmd.Execute()
}

func project(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\nEXPOSE 8080\n"), 0o600))

	return dir
}

func TestCmd_WritesTheManifest(t *testing.T) {
	dir := project(t)

	stdout, stderr, err := runCmd(t, "--dir", dir, "--yes", "--name", "my-app")
	require.NoError(t, err)

	path := manifest.Path(dir)

	// stdout is the path and nothing else, so it can be piped.
	assert.Equal(t, path+"\n", stdout.String())
	assert.Contains(t, stderr.String(), "Wrote")
	assert.FileExists(t, path)
}

// JSON mode means stdout is JSON and only JSON: every human-facing word goes
// to stderr, including the wizard that JSON mode suppresses entirely.
func TestCmd_JSONEnvelope(t *testing.T) {
	dir := project(t)

	stdout, _, err := runCmd(t, "--dir", dir, "--name", "my-app", "--output-format", "json")
	require.NoError(t, err)

	var envelope struct {
		Config struct {
			Path       string `json:"path"`
			Name       string `json:"name"`
			WorkloadID string `json:"workloadId"`
			CreateOnUp bool   `json:"createOnUp"`
			BuildMode  string `json:"buildMode"`
			Action     string `json:"action"`
		} `json:"config"`
	}

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &envelope))

	assert.Equal(t, manifest.Path(dir), envelope.Config.Path)
	assert.Equal(t, "my-app", envelope.Config.Name)
	assert.Empty(t, envelope.Config.WorkloadID)
	assert.True(t, envelope.Config.CreateOnUp)
	assert.Equal(t, manifest.BuildModeDockerfile, envelope.Config.BuildMode)
	assert.Equal(t, "created", envelope.Config.Action)
}

func TestCmd_ExistingManifestReportsUnchanged(t *testing.T) {
	dir := project(t)
	require.NoError(t, os.WriteFile(manifest.Path(dir), []byte("name: already-here\nartifactId: 68b0\n"), 0o600))

	stdout, _, err := runCmd(t, "--dir", dir, "--name", "ignored", "--output-format", "json")
	require.NoError(t, err)

	var envelope struct {
		Config struct {
			Action string `json:"action"`
		} `json:"config"`
	}

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &envelope))
	assert.Equal(t, "unchanged", envelope.Config.Action)
}

func TestCmd_DryRunWritesNothing(t *testing.T) {
	dir := project(t)

	_, stderr, err := runCmd(t, "--dir", dir, "--yes", "--name", "my-app", "--dry-run")
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "Dry run")
	assert.Contains(t, stderr.String(), "name: my-app")
	assert.NoFileExists(t, manifest.Path(dir))
}

// The flag exists so pointing it elsewhere fails here, with the reason,
// rather than being quietly ignored.
func TestCmd_DockerfileFlagIsPinned(t *testing.T) {
	dir := project(t)

	_, _, err := runCmd(t, "--dir", dir, "--yes", "--name", "my-app", "--dockerfile", "docker/Dockerfile")
	require.Error(t, err)

	assert.Contains(t, err.Error(), "is not supported")
	assert.NoFileExists(t, manifest.Path(dir))

	// The value it does accept is the one it defaults to.
	_, _, err = runCmd(t, "--dir", dir, "--yes", "--name", "my-app", "--dockerfile", "./Dockerfile")
	require.NoError(t, err)
}

func TestCmd_RejectsBadFlagCombinations(t *testing.T) {
	tests := map[string][]string{
		"id and name":       {"--workload-id", "68b0", "--name", "my-app"},
		"unsupported type":  {"--name", "my-app", "--type", "nim"},
		"bad build mode":    {"--name", "my-app", "--build-mode", "magic"},
		"relative health":   {"--name", "my-app", "--health", "health"},
		"privileged port":   {"--name", "my-app", "--port", "80"},
		"image mode no uri": {"--name", "my-app", "--build-mode", "image"},
		"probe and no probe": {
			"--name", "my-app", "--no-readiness-probe", "--health", "/ready",
		},
		"unsupported importance": {"--name", "my-app", "--importance", "urgent"},
	}

	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			// A directory each: sharing one means a case that stops erroring
			// writes a manifest the remaining cases then find already there,
			// and they all fail without naming the one that regressed.
			dir := project(t)

			_, _, err := runCmd(t, append([]string{"--dir", dir, "--yes"}, args...)...)
			require.Error(t, err)
			assert.NoFileExists(t, manifest.Path(dir))
		})
	}
}

// The two fields the settings screen hides have to stay reachable without a
// terminal, which for the probe means a flag of its own: an empty --health
// cannot be told apart from an absent one.
func TestCmd_HiddenFieldsAreReachableByFlag(t *testing.T) {
	dir := project(t)

	_, _, err := runCmd(t, "--dir", dir, "--yes", "--name", "my-app",
		"--no-readiness-probe", "--importance", "high")
	require.NoError(t, err)

	written, err := os.ReadFile(manifest.Path(dir))
	require.NoError(t, err)

	assert.NotContains(t, string(written), "readinessProbe")
	assert.Contains(t, string(written), "importance: high")
}

func TestCmd_RejectsPositionalArgs(t *testing.T) {
	_, _, err := runCmd(t, "extra")
	require.Error(t, err)

	assert.Contains(t, err.Error(), "unknown command")
}

func TestCmd_InvalidOutputFormat(t *testing.T) {
	_, _, err := runCmd(t, "--output-format", "yaml")
	require.Error(t, err)

	assert.Contains(t, err.Error(), `invalid output format "yaml"`)
}

// JSON output must suppress the wizard outright. A wizard drawing alongside a
// machine-readable stdout is a trap for whoever is parsing it, and the guard
// is invisible under `go test` unless the terminal seam is forced.
func TestCmd_JSONImpliesNonInteractive(t *testing.T) {
	restoreTerminal := wizard.SetStdinTerminalForTest(true)
	defer restoreTerminal()

	var ran bool

	restoreFlow := wizard.SetInteractiveFlowForTest(
		func(wizard.Options, wizard.Detected) ([]byte, manifest.Draft, string, error) {
			ran = true

			return nil, manifest.Draft{}, "", errors.New("the wizard must not run")
		})
	defer restoreFlow()

	_, _, err := runCmd(t, "--dir", project(t), "--name", "my-app", "--output-format", "json")
	require.NoError(t, err)
	assert.False(t, ran, "--output-format json must not open the wizard")

	// The same run without JSON does reach it, so the assertion above is not
	// passing for some unrelated reason.
	ran = false
	_, _, err = runCmd(t, "--dir", project(t), "--name", "my-app")
	require.Error(t, err)
	assert.True(t, ran, "an interactive run with a terminal opens the wizard")
}

// --yes suppresses it too, terminal or not.
func TestCmd_YesImpliesNonInteractive(t *testing.T) {
	restoreTerminal := wizard.SetStdinTerminalForTest(true)
	defer restoreTerminal()

	var ran bool

	restoreFlow := wizard.SetInteractiveFlowForTest(
		func(wizard.Options, wizard.Detected) ([]byte, manifest.Draft, string, error) {
			ran = true

			return nil, manifest.Draft{}, "", errors.New("the wizard must not run")
		})
	defer restoreFlow()

	_, _, err := runCmd(t, "--dir", project(t), "--yes", "--name", "my-app")
	require.NoError(t, err)
	assert.False(t, ran)
}

// projectWithEnv is a buildable project carrying a .env, which is what puts
// the classifier and the reporting below in play.
func projectWithEnv(t *testing.T, env string) string {
	t.Helper()

	dir := project(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, wizard.EnvFileName), []byte(env), 0o600))

	return dir
}

// A pipeline reading only the envelope has to be able to see that the file it
// just generated names a placeholder rather than a credential, or it will
// deploy something that cannot start.
func TestCmd_JSONEnvelopeCarriesTheEnvCounts(t *testing.T) {
	dir := projectWithEnv(t, "LOG_LEVEL=debug\nAPI_TOKEN=sk-live-51H8xQvZmNpKdRt7YwLbG3JfA9\n")

	stdout, _, err := runCmd(t, "--dir", dir, "--name", "my-app", "--output-format", "json")
	require.NoError(t, err)

	var envelope struct {
		Config struct {
			EnvKeysListed     int      `json:"envKeysListed"`
			EnvSecretsPending int      `json:"envSecretsPending"`
			EnvLiterals       []string `json:"envLiterals"`
		} `json:"config"`
	}

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &envelope))

	assert.Equal(t, 2, envelope.Config.EnvKeysListed)
	assert.Equal(t, 1, envelope.Config.EnvSecretsPending)
	assert.Equal(t, []string{"LOG_LEVEL"}, envelope.Config.EnvLiterals)
}

// The classifier prefers to call a doubtful value secret, but it is a
// heuristic and nothing prompts on a headless run. Naming what went in as a
// literal is the only chance to catch a misread before the file is committed.
func TestCmd_NamesTheValuesWrittenInTheClear(t *testing.T) {
	dir := projectWithEnv(t, "LOG_LEVEL=debug\nREGION=eu-west-1\nAPI_TOKEN=sk-live-51H8xQvZmNpKdRt7YwLbG3JfA9\n")

	_, stderr, err := runCmd(t, "--dir", dir, "--yes", "--name", "my-app")
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "Values written in the clear: LOG_LEVEL, REGION.")
	assert.Contains(t, stderr.String(), manifest.CredentialShorthandPrefix)

	// The secret is named nowhere, and its value nowhere at all.
	assert.NotContains(t, stderr.String(), "in the clear: LOG_LEVEL, REGION, API_TOKEN")
	assert.NotContains(t, stderr.String(), "sk-live-51H8xQvZmNpKdRt7YwLbG3JfA9")
}

// A long .env would bury the summary the listing qualifies, so it is capped
// and the remainder counted.
func TestCmd_CapsTheListOfLiterals(t *testing.T) {
	var env strings.Builder

	for i := range 12 {
		fmt.Fprintf(&env, "PLAIN_%d=value%d\n", i, i)
	}

	_, stderr, err := runCmd(t, "--dir", projectWithEnv(t, env.String()), "--yes", "--name", "my-app")
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "PLAIN_7 and 4 more.")
	assert.NotContains(t, stderr.String(), "PLAIN_8")
}

// A .env the parser chokes on still reaches the user as a warning. The
// parser's own message carries the rest of the file, values included, so the
// property under test is that none of it survives to stderr.
func TestCmd_UnparseableEnvFileLeaksNoValues(t *testing.T) {
	dir := projectWithEnv(t, "GOOD=1\nBAD-NAME=x\nAPI_TOKEN=sk-live-do-not-print\nDB_PASSWORD=hunter2\n")

	_, stderr, err := runCmd(t, "--dir", dir, "--yes", "--name", "my-app")
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "Warning:")
	assert.Contains(t, stderr.String(), "on line 2")
	assert.NotContains(t, stderr.String(), "sk-live-do-not-print")
	assert.NotContains(t, stderr.String(), "hunter2")
}
