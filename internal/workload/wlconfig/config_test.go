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

package wlconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefault_IsFullyPopulatedProvidedMode(t *testing.T) {
	cfg := Default("app")

	assert.Equal(t, "app", cfg.Name)
	assert.Equal(t, ModeProvided, cfg.BuildMode())
	require.NotNil(t, cfg.Build)
	assert.Equal(t, DefaultDockerfile, cfg.Build.Dockerfile)
	assert.Equal(t, DefaultPort, cfg.Build.Port)
	require.NotNil(t, cfg.Runtime)
	assert.InDelta(t, DefaultCPU, cfg.Runtime.CPU, 0.0001)
}

func TestSaveLoadRoundTrip_Default(t *testing.T) {
	dir := t.TempDir()
	cfg := Default("my-app")

	require.NoError(t, Save(dir, cfg))

	got, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, cfg, got)
}

func TestSave_WritesCommentedManifest(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, Save(dir, Default("app")))

	data, err := os.ReadFile(Path(dir))
	require.NoError(t, err)

	s := string(data)
	assert.Contains(t, s, "# DataRobot workload manifest")
	assert.Contains(t, s, "name: app")
	assert.Contains(t, s, "build:")
	assert.Contains(t, s, "dockerfile: ./Dockerfile")
	assert.Contains(t, s, "runtime:")
	assert.Contains(t, s, "cpu: 0.5")
}

func TestSave_WorkloadIDWriteBackKeepsManifest(t *testing.T) {
	dir := t.TempDir()
	cfg := Default("app")

	require.NoError(t, Save(dir, cfg))

	// Simulate `up` recording the id after create.
	cfg.WorkloadID = "wl-1"
	require.NoError(t, Save(dir, cfg))

	data, err := os.ReadFile(Path(dir))
	require.NoError(t, err)
	assert.Contains(t, string(data), "workloadId: wl-1")
	assert.Contains(t, string(data), "dockerfile:", "manifest fields survive the write-back")

	got, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "wl-1", got.WorkloadID)
	assert.Equal(t, "app", got.Name)
}

func TestSave_IsAtomicAndLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, Save(dir, Default("app")))

	entries, err := os.ReadDir(Dir(dir))
	require.NoError(t, err)

	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".tmp", "temp file should be renamed away")
	}
}

func TestBuildMode(t *testing.T) {
	assert.Equal(t, ModeProvided, Config{Build: &Build{Dockerfile: "./Dockerfile"}}.BuildMode())
	assert.Equal(t, ModeGenerated, Config{Build: &Build{ExecutionEnvironment: "ee"}}.BuildMode())
	assert.Equal(t, ModeGenerated, Config{}.BuildMode())
	assert.Equal(t, ModeImage, Config{Build: &Build{Image: "repo/app:1"}}.BuildMode())
	assert.Equal(t, ModeImage, Config{Build: &Build{Image: "repo/app:1", Dockerfile: "./Dockerfile"}}.BuildMode(),
		"image wins over dockerfile so switching modes only needs the image line")
}

func TestSaveLoadRoundTrip_ImageModeWithEnv(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Name:       "agent-service",
		Importance: "high",
		Build:      &Build{Image: "otkachnlp/fastapi-server-example:latest", Entrypoint: []string{"python", "server.py"}, Port: 8080, Health: "/readyz"},
		Runtime:    &Runtime{Replicas: 1, CPU: 1, Memory: "512MB"},
		Env: map[string]string{
			"MODEL":               "azure/gpt-5-nano-2025-08-07",
			"DATAROBOT_API_TOKEN": "${DATAROBOT_API_TOKEN}",
		},
	}

	require.NoError(t, Save(dir, cfg))

	got, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, cfg, got)
	assert.Equal(t, ModeImage, got.BuildMode())
}

func TestMarshal_ImageModeSection(t *testing.T) {
	cfg := Config{
		Name:  "app",
		Build: &Build{Image: "repo/app:1", Entrypoint: []string{"python", "server.py"}, Port: 8080, Health: "/readyz"},
	}

	s := string(Marshal(cfg))
	assert.Contains(t, s, "image: repo/app:1")
	assert.Contains(t, s, `"python", "server.py"`)
	assert.Contains(t, s, "health: /readyz")
	// No active dockerfile/EE lines; they may appear only as commented hints.
	assert.NotContains(t, s, "\n  dockerfile:")
	assert.NotContains(t, s, "\n  executionEnvironment:")
}

func TestWithDefaults_ImageModeDoesNotInjectDockerfile(t *testing.T) {
	cfg := Config{Name: "app", Build: &Build{Image: "repo/app:1"}}

	out := cfg.withDefaults()
	assert.Empty(t, out.Build.Dockerfile)
	assert.Equal(t, ModeImage, out.BuildMode())
}

func TestMarshal_EnvSectionRendersSorted(t *testing.T) {
	cfg := Config{Name: "app", Env: map[string]string{"B_VAR": "2", "A_VAR": "1"}}

	s := string(Marshal(cfg))
	assert.Contains(t, s, "env:\n  A_VAR: \"1\"\n  B_VAR: \"2\"\n")
}

func TestMarshal_GeneratedModeShowsEEAndEntrypoint(t *testing.T) {
	cfg := Config{
		Name:  "app",
		Build: &Build{ExecutionEnvironment: "my-ee", Entrypoint: []string{"uvicorn", "app:app"}, Port: 9000, Health: "/h"},
	}

	s := string(Marshal(cfg))
	assert.Contains(t, s, `executionEnvironment: "my-ee"`)
	assert.Contains(t, s, `"uvicorn", "app:app"`)
	// The dockerfile line appears only as a commented alternative.
	assert.NotContains(t, s, "\n  dockerfile:")
}

func TestLoad_MissingReturnsErrNotConfigured(t *testing.T) {
	dir := t.TempDir()

	_, err := Load(dir)
	require.ErrorIs(t, err, ErrNotConfigured)
	assert.False(t, Exists(dir))
}

func TestLoad_ParsesManifest(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, DirName), 0o755))

	manifest := "name: svc\nimportance: high\nbuild:\n  dockerfile: ./Dockerfile\n  port: 9000\nruntime:\n  cpu: 2\n  memory: 1GB\n"
	require.NoError(t, os.WriteFile(Path(dir), []byte(manifest), 0o644))

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "svc", cfg.Name)
	assert.Equal(t, "high", cfg.Importance)
	assert.Equal(t, 9000, cfg.Build.Port)
	assert.InDelta(t, 2.0, cfg.Runtime.CPU, 0.0001)
	assert.Equal(t, "1GB", cfg.Runtime.Memory)
}

func TestLoad_MalformedYAMLErrors(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, DirName), 0o755))
	require.NoError(t, os.WriteFile(Path(dir), []byte("name: [unterminated\n"), 0o644))

	_, err := Load(dir)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrNotConfigured)
	assert.Contains(t, err.Error(), "parse")
}
