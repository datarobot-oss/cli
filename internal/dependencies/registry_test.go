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

package dependencies

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeLookPath returns a lookPath function that reports the given set of
// executables as present and everything else as absent.
func fakeLookPath(present ...string) func(string) (string, error) {
	set := make(map[string]bool, len(present))

	for _, name := range present {
		set[name] = true
	}

	return func(name string) (string, error) {
		if set[name] {
			return "/usr/bin/" + name, nil
		}

		return "", fmt.Errorf("%s: not found", name)
	}
}

// fakeGetenv returns a getenv function that answers from the supplied key→value map.
func fakeGetenv(env map[string]string) func(string) string {
	return func(key string) string {
		return env[key]
	}
}

// noDirExists is a dirExists stub that always returns false.
func noDirExists(_ string) bool { return false }

// ──────────────────────────────────────────────────────────────
// NormalizeToolName
// ──────────────────────────────────────────────────────────────

func TestNormalizeToolName(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"python", "python"},
		{"Python", "python"},
		{"python3", "python"},
		{"py3", "python"},
		{"py", "python"},
		{"python@3", "python"},
		{"uv", "uv"},
		{"UV", "uv"},
		{"node", "node"},
		{"node.js", "node"},
		{"Node.js", "node"},
		{"nodejs", "node"},
		{"task", "task"},
		{"Taskfile task runner", "task"},
		{"TASKFILE TASK RUNNER", "task"},
		{"pulumi", "pulumi"},
		{"Pulumi infrastructure as code tool", "pulumi"},
		{"git", "git"},
		{"Git source control management tool", "git"},
		{"  python  ", "python"},
		{"unknown", ""},
		{"", ""},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, NormalizeToolName(tc.input))
		})
	}
}

// ──────────────────────────────────────────────────────────────
// DetectEnvironment
// ──────────────────────────────────────────────────────────────

func TestDetectEnvironment_BrewPresentOnDarwin(t *testing.T) {
	env := detectEnvironment(fakeLookPath("brew"), fakeGetenv(nil), noDirExists, "darwin")

	assert.True(t, env["brew"])
	assert.False(t, env["winget"])
	assert.False(t, env["is_windows"])
}

func TestDetectEnvironment_BrewSuppressedOnWindows(t *testing.T) {
	// Even if brew is on PATH, it must not be treated as available on Windows.
	env := detectEnvironment(fakeLookPath("brew", "winget", "choco"), fakeGetenv(nil), noDirExists, "windows")

	assert.False(t, env["brew"])
	assert.True(t, env["winget"])
	assert.True(t, env["choco"])
	assert.True(t, env["is_windows"])
}

func TestDetectEnvironment_PyenvDetected(t *testing.T) {
	env := detectEnvironment(fakeLookPath("pyenv"), fakeGetenv(nil), noDirExists, "linux")

	assert.True(t, env["pyenv"])
	assert.False(t, env["brew"])
}

func TestDetectEnvironment_NVM_ViaEnvVar(t *testing.T) {
	getenv := fakeGetenv(map[string]string{"NVM_DIR": "/home/user/.nvm"})
	dirExists := func(p string) bool { return p == "/home/user/.nvm" }

	env := detectEnvironment(fakeLookPath(), getenv, dirExists, "linux")

	assert.True(t, env["nvm"])
}

func TestDetectEnvironment_NVM_ViaHomeFallback(t *testing.T) {
	getenv := fakeGetenv(map[string]string{"HOME": "/home/user"})
	dirExists := func(p string) bool { return p == "/home/user/.nvm" }

	env := detectEnvironment(fakeLookPath(), getenv, dirExists, "linux")

	assert.True(t, env["nvm"])
}

func TestDetectEnvironment_NVM_AbsentOnWindows(t *testing.T) {
	getenv := fakeGetenv(map[string]string{"NVM_DIR": "/home/user/.nvm"})
	dirExists := func(p string) bool { return true }

	// nvm must never be reported on Windows even if the directory exists.
	env := detectEnvironment(fakeLookPath(), getenv, dirExists, "windows")

	assert.False(t, env["nvm"])
}

func TestDetectEnvironment_AllAbsent(t *testing.T) {
	env := detectEnvironment(fakeLookPath(), fakeGetenv(nil), noDirExists, "linux")

	for key, val := range env {
		if key == "is_windows" {
			continue
		}

		assert.False(t, val, "expected %q to be false when nothing is installed", key)
	}
}

// ──────────────────────────────────────────────────────────────
// GetInstallSuggestion
// ──────────────────────────────────────────────────────────────

// TestGetInstallSuggestion_PyenvPresentBrewAbsent_UV is the acceptance-criteria test:
// with pyenv on PATH and brew absent, GetInstallSuggestion("uv") must return the
// pyenv-based command (pip install uv).
func TestGetInstallSuggestion_PyenvPresentBrewAbsent_UV(t *testing.T) {
	env := map[string]bool{
		"pyenv": true,
		"brew":  false,
	}

	result := getInstallSuggestion("uv", env, "linux")

	require.NotEmpty(t, result)
	assert.Equal(t, []string{"pip install uv"}, result)
}

func TestGetInstallSuggestion_BrewPresent_UV(t *testing.T) {
	env := map[string]bool{"brew": true}

	result := getInstallSuggestion("uv", env, "darwin")

	require.NotEmpty(t, result)
	assert.Equal(t, []string{"brew install uv"}, result)
}

func TestGetInstallSuggestion_BrewPresent_Python(t *testing.T) {
	env := map[string]bool{"brew": true}

	result := getInstallSuggestion("python", env, "darwin")

	require.NotEmpty(t, result)
	assert.Equal(t, []string{"brew install python@3.12"}, result)
}

func TestGetInstallSuggestion_FallbackUnix_WhenNoManagerDetected(t *testing.T) {
	env := map[string]bool{}

	result := getInstallSuggestion("uv", env, "linux")

	require.NotEmpty(t, result)
	assert.Contains(t, result[0], "curl")
}

func TestGetInstallSuggestion_FallbackWindows(t *testing.T) {
	env := map[string]bool{"is_windows": true}

	result := getInstallSuggestion("uv", env, "windows")

	require.NotEmpty(t, result)
	assert.Contains(t, result[0], "iex")
}

func TestGetInstallSuggestion_UnknownTool(t *testing.T) {
	env := map[string]bool{"brew": true}

	result := getInstallSuggestion("nonexistent-tool", env, "linux")

	assert.Nil(t, result)
}

func TestGetInstallSuggestion_WingetPresent_Task(t *testing.T) {
	env := map[string]bool{"winget": true, "is_windows": true}

	result := getInstallSuggestion("task", env, "windows")

	require.NotEmpty(t, result)
	assert.Equal(t, []string{"winget install Task.Task"}, result)
}

func TestGetInstallSuggestion_NVMPresent_Node(t *testing.T) {
	env := map[string]bool{"nvm": true}

	result := getInstallSuggestion("node", env, "linux")

	require.NotEmpty(t, result)
	assert.Equal(t, []string{"nvm install 24", "nvm use 24"}, result)
}

func TestGetInstallSuggestion_AllToolsHaveAtLeastOneFallback(t *testing.T) {
	emptyEnv := map[string]bool{}

	for key := range ToolRegistry {
		t.Run(key, func(t *testing.T) {
			result := getInstallSuggestion(key, emptyEnv, "linux")
			assert.NotNil(t, result, "tool %q must have a fallback strategy", key)
		})
	}
}
