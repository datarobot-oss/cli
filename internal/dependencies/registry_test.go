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
	t.Setenv("HOME", "/home/user")

	dirExists := func(p string) bool { return p == "/home/user/.nvm" }

	env := detectEnvironment(fakeLookPath(), fakeGetenv(nil), dirExists, "linux")

	assert.True(t, env["nvm"])
}

func TestDetectEnvironment_NVM_HomeDirError_NVMAbsent(t *testing.T) {
	// Unset HOME so os.UserHomeDir() cannot resolve a home directory;
	// nvm must be reported absent rather than panicking.
	t.Setenv("HOME", "")

	env := detectEnvironment(fakeLookPath(), fakeGetenv(nil), noDirExists, "linux")

	assert.False(t, env["nvm"])
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
// majorMinorVersion
// ──────────────────────────────────────────────────────────────

func TestMajorMinorVersion(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"3.9.6", "3.9"},
		{"24.0.0", "24.0"},
		{"1.2.3", "1.2"},
		{"3", "3"},
		{"", ""},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, majorMinorVersion(tc.input))
		})
	}
}

// ──────────────────────────────────────────────────────────────
// substituteCmds
// ──────────────────────────────────────────────────────────────

func TestSubstituteCmds_EmptySlice(t *testing.T) {
	assert.Empty(t, substituteCmds(nil, "3.9.6"))
}

func TestSubstituteCmds_VersionPlaceholder(t *testing.T) {
	result := substituteCmds([]string{"pyenv install {version}", "pyenv global {version}"}, "3.9.6")

	assert.Equal(t, []string{"pyenv install 3.9.6", "pyenv global 3.9.6"}, result)
}

func TestSubstituteCmds_VersionMmPlaceholder(t *testing.T) {
	result := substituteCmds([]string{"brew install python@{version_mm}"}, "3.9.6")

	assert.Equal(t, []string{"brew install python@3.9"}, result)
}

func TestSubstituteCmds_BothPlaceholders_OrderSafe(t *testing.T) {
	// {version_mm} must be replaced before {version} to avoid partial match.
	// If {version} were replaced first, "python@{version_mm}" would become
	// "python@{3.9.6_mm}" (corrupted) rather than "python@3.9".
	result := substituteCmds([]string{"pyenv install {version}", "brew install python@{version_mm}"}, "3.9.6")

	assert.Equal(t, []string{"pyenv install 3.9.6", "brew install python@3.9"}, result)
}

func TestSubstituteCmds_NoPlaceholders(t *testing.T) {
	cmds := []string{"brew install uv", "pip install uv"}
	result := substituteCmds(cmds, "3.9.6")

	assert.Equal(t, cmds, result)
}

// ──────────────────────────────────────────────────────────────
// getStrategyTip — ManagerStrategy
// ──────────────────────────────────────────────────────────────

func TestManagerStrategy_GetStrategyTip_SingleCommand(t *testing.T) {
	ms := ManagerStrategy{Manager: "pyenv", Commands: []string{"pip install uv"}}

	assert.Equal(t, "  Tip: You have pyenv — try: pip install uv", ms.getStrategyTip("linux"))
}

func TestManagerStrategy_GetStrategyTip_MultipleCommands(t *testing.T) {
	ms := ManagerStrategy{
		Manager:  "asdf",
		Commands: []string{"asdf install uv latest", "asdf global uv latest"},
	}

	assert.Equal(t, TAB+"Tip: You have asdf — try: \n"+TAB+TAB+"asdf install uv latest\n"+TAB+TAB+"asdf global uv latest", ms.getStrategyTip("linux"))
}

func TestManagerStrategy_GetStrategyTip_GoosIgnored(t *testing.T) {
	ms := ManagerStrategy{Manager: "brew", Commands: []string{"brew install uv"}}

	assert.Equal(t, ms.getStrategyTip("linux"), ms.getStrategyTip("windows"), "goos must not affect ManagerStrategy tip")
}

// ──────────────────────────────────────────────────────────────
// getStrategyTip — FallbackStrategy
// ──────────────────────────────────────────────────────────────

func TestFallbackStrategy_GetStrategyTip_SingleCommand(t *testing.T) {
	fs := FallbackStrategy{Commands: []string{"curl -LsSf https://astral.sh/uv/install.sh | sh"}}

	assert.Equal(t, "  Try: curl -LsSf https://astral.sh/uv/install.sh | sh", fs.getStrategyTip("linux"))
}

func TestFallbackStrategy_GetStrategyTip_MultipleCommands(t *testing.T) {
	fs := FallbackStrategy{Commands: []string{"curl https://pyenv.run | bash", "pyenv install 3.12", "pyenv global 3.12"}}

	assert.Equal(t, TAB+"Try:\n"+TAB+TAB+"curl https://pyenv.run | bash\n"+TAB+TAB+"pyenv install 3.12\n"+TAB+TAB+"pyenv global 3.12", fs.getStrategyTip("linux"))
}

func TestFallbackStrategy_GetStrategyTip_URLOnly(t *testing.T) {
	fs := FallbackStrategy{URL: "https://git-scm.com/downloads"}

	assert.Equal(t, "  See: https://git-scm.com/downloads", fs.getStrategyTip("linux"))
}

func TestFallbackStrategy_GetStrategyTip_Empty(t *testing.T) {
	assert.Empty(t, FallbackStrategy{}.getStrategyTip("linux"))
}

func TestFallbackStrategy_GetStrategyTip_WindowsOverride(t *testing.T) {
	fs := FallbackStrategy{
		Commands:        []string{"curl -LsSf https://astral.sh/uv/install.sh | sh"},
		CommandsWindows: []string{`powershell -c "irm https://astral.sh/uv/install.ps1 | iex"`},
	}

	assert.Contains(t, fs.getStrategyTip("windows"), "iex")
	assert.Contains(t, fs.getStrategyTip("linux"), "curl")
}

// ──────────────────────────────────────────────────────────────
// selectInstallStrategy
// ──────────────────────────────────────────────────────────────

// TestSelectInstallStrategy_PyenvPresentBrewAbsent_UV is the acceptance-criteria test:
// with pyenv on PATH and brew absent, selectInstallStrategy("uv") must return the
// pyenv ManagerStrategy (pip install uv).
func TestSelectInstallStrategy_PyenvPresentBrewAbsent_UV(t *testing.T) {
	env := map[string]bool{
		"pyenv": true,
		"brew":  false,
	}

	ms, ok := selectInstallStrategy("uv", "", env).(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, "pyenv", ms.Manager)
	assert.Equal(t, []string{"pip install uv"}, ms.Commands)
}

func TestSelectInstallStrategy_BrewPresent_UV(t *testing.T) {
	env := map[string]bool{"brew": true}

	ms, ok := selectInstallStrategy("uv", "", env).(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, []string{"brew install uv"}, ms.Commands)
}

func TestSelectInstallStrategy_BrewPresent_Python(t *testing.T) {
	env := map[string]bool{"brew": true}

	ms, ok := selectInstallStrategy("python", "", env).(ManagerStrategy)

	require.True(t, ok)
	// Without version the placeholder is returned as-is; callers call .withVersion().
	assert.Equal(t, []string{"brew install python@{version_mm}"}, ms.Commands)
}

func TestSelectInstallStrategy_FallbackUnix_WhenNoManagerDetected(t *testing.T) {
	env := map[string]bool{}

	fs, ok := selectInstallStrategy("uv", "", env).(FallbackStrategy)

	require.True(t, ok)
	require.NotEmpty(t, fs.Commands)
	assert.Contains(t, fs.Commands[0], "curl")
}

func TestSelectInstallStrategy_FallbackWindows(t *testing.T) {
	env := map[string]bool{"is_windows": true}

	fs, ok := selectInstallStrategy("uv", "", env).(FallbackStrategy)

	require.True(t, ok)
	require.NotEmpty(t, fs.CommandsWindows)
	assert.Contains(t, fs.CommandsWindows[0], "iex")
}

func TestSelectInstallStrategy_UnknownTool(t *testing.T) {
	env := map[string]bool{"brew": true}

	result := selectInstallStrategy("nonexistent-tool", "", env)

	assert.Nil(t, result)
}

func TestSelectInstallStrategy_WingetPresent_Task(t *testing.T) {
	env := map[string]bool{"winget": true, "is_windows": true}

	ms, ok := selectInstallStrategy("task", "", env).(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, []string{"winget install Task.Task"}, ms.Commands)
}

func TestSelectInstallStrategy_NVMPresent_Node(t *testing.T) {
	env := map[string]bool{"nvm": true}

	ms, ok := selectInstallStrategy("node", "", env).(ManagerStrategy)

	require.True(t, ok)
	// Without version the placeholder is returned as-is; callers call .withVersion().
	assert.Equal(t, []string{"nvm install {version}", "nvm use {version}"}, ms.Commands)
}

func TestSelectInstallStrategy_AllToolsHaveAtLeastOneFallback(t *testing.T) {
	emptyEnv := map[string]bool{}

	for key := range ToolRegistry {
		t.Run(key, func(t *testing.T) {
			result := selectInstallStrategy(key, "", emptyEnv)
			assert.NotNil(t, result, "tool %q must have a fallback strategy", key)
		})
	}
}

func TestSelectInstallStrategy_SkipsFailedMgr(t *testing.T) {
	env := map[string]bool{"pyenv": true, "brew": true}

	ms, ok := selectInstallStrategy("uv", "pyenv", env).(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, "brew", ms.Manager, "should skip pyenv and return brew")
}

// ──────────────────────────────────────────────────────────────
// withVersion
// ──────────────────────────────────────────────────────────────

func TestWithVersion_ManagerStrategy_Pyenv_Python(t *testing.T) {
	env := map[string]bool{"pyenv": true}

	ms, ok := selectInstallStrategy("python", "", env).(ManagerStrategy)
	require.True(t, ok)

	result, ok := ms.withVersion("3.9.6").(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, []string{"pyenv install 3.9.6", "pyenv global 3.9.6"}, result.Commands)
}

func TestWithVersion_ManagerStrategy_Asdf_Python(t *testing.T) {
	env := map[string]bool{"asdf": true}

	ms, ok := selectInstallStrategy("python", "", env).(ManagerStrategy)
	require.True(t, ok)

	result, ok := ms.withVersion("3.9.6").(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, []string{"asdf install python 3.9.6", "asdf global python 3.9.6"}, result.Commands)
}

func TestWithVersion_ManagerStrategy_Brew_Python(t *testing.T) {
	env := map[string]bool{"brew": true}

	ms, ok := selectInstallStrategy("python", "", env).(ManagerStrategy)
	require.True(t, ok)

	result, ok := ms.withVersion("3.9.6").(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, []string{"brew install python@3.9"}, result.Commands)
}

func TestWithVersion_ManagerStrategy_EmptyVersion_UsesDefault(t *testing.T) {
	// python/pyenv has DefaultVersion "3.14" — used when MinimumVersion is empty.
	env := map[string]bool{"pyenv": true}

	ms, ok := selectInstallStrategy("python", "", env).(ManagerStrategy)
	require.True(t, ok)

	result, ok := ms.withVersion("").(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, []string{"pyenv install 3.14", "pyenv global 3.14"}, result.Commands)
}

func TestWithVersion_FallbackStrategy_Python(t *testing.T) {
	fs, ok := selectInstallStrategy("python", "", map[string]bool{}).(FallbackStrategy)
	require.True(t, ok)

	result, ok := fs.withVersion("3.9.6").(FallbackStrategy)

	require.True(t, ok)
	assert.Contains(t, result.Commands, "pyenv install 3.9.6")
	assert.Contains(t, result.Commands, "pyenv global 3.9.6")
}

func TestWithVersion_FallbackStrategy_WindowsCommandsAlsoSubstituted(t *testing.T) {
	fs, ok := selectInstallStrategy("python", "", map[string]bool{}).(FallbackStrategy)
	require.True(t, ok)

	result, ok := fs.withVersion("3.9.6").(FallbackStrategy)

	require.True(t, ok)
	assert.Contains(t, result.CommandsWindows, "pyenv install 3.9.6")
	assert.Contains(t, result.CommandsWindows, "pyenv global 3.9.6")
}

func TestWithVersion_FallbackStrategy_URLOnlyUnchanged(t *testing.T) {
	// git has a URL-only FallbackStrategy — withVersion must not panic and must
	// leave the URL intact.
	fs, ok := selectInstallStrategy("git", "", map[string]bool{}).(FallbackStrategy)
	require.True(t, ok)

	result, ok := fs.withVersion("2.40.0").(FallbackStrategy)

	require.True(t, ok)
	assert.Equal(t, "https://git-scm.com/downloads", result.URL)
	assert.Empty(t, result.Commands)
}

func TestWithVersion_ManagerStrategy_NoPlaceholders(t *testing.T) {
	// uv's brew strategy has no version placeholders — commands must be returned as-is.
	env := map[string]bool{"brew": true}

	ms, ok := selectInstallStrategy("uv", "", env).(ManagerStrategy)
	require.True(t, ok)

	result, ok := ms.withVersion("0.11.20").(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, []string{"brew install uv"}, result.Commands)
}

func TestWithVersion_ManagerStrategy_EmptyVersion_NoDefaultVersion_NoPlaceholders(t *testing.T) {
	// brew/node has no DefaultVersion and no {version} placeholder — commands pass
	// through substituteCmds unchanged when both version and DefaultVersion are empty.
	env := map[string]bool{"brew": true}

	ms, ok := selectInstallStrategy("node", "", env).(ManagerStrategy)
	require.True(t, ok)

	result, ok := ms.withVersion("").(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, []string{"brew install node"}, result.Commands)
}

func TestSelectInstallStrategy_DisplayNameNormalized(t *testing.T) {
	// selectInstallStrategy calls NormalizeToolName internally, so display names
	// like "Python" (capital P) must resolve to the "python" registry entry.
	env := map[string]bool{"brew": true}

	ms, ok := selectInstallStrategy("Python", "", env).(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, "brew", ms.Manager)
}

func TestSelectInstallStrategy_FailedMgrIsOnlyDetectedMgr_FallsToFallback(t *testing.T) {
	// brew is the only detected manager but it already failed — must skip it and
	// return the FallbackStrategy rather than nil.
	env := map[string]bool{"brew": true}

	_, ok := selectInstallStrategy("uv", "brew", env).(FallbackStrategy)

	require.True(t, ok)
}

func TestWithVersion_ManagerStrategy_DefaultVersion_UsedWhenVersionEmpty(t *testing.T) {
	// nvm strategy for node has DefaultVersion "24" — used when MinimumVersion is empty.
	env := map[string]bool{"nvm": true}

	ms, ok := selectInstallStrategy("node", "", env).(ManagerStrategy)
	require.True(t, ok)

	result, ok := ms.withVersion("").(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, []string{"nvm install 24", "nvm use 24"}, result.Commands)
}

func TestWithVersion_ManagerStrategy_MinimumVersionOverridesDefault(t *testing.T) {
	// When MinimumVersion is set, it takes precedence over DefaultVersion.
	env := map[string]bool{"nvm": true}

	ms, ok := selectInstallStrategy("node", "", env).(ManagerStrategy)
	require.True(t, ok)

	result, ok := ms.withVersion("20.0.0").(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, []string{"nvm install 20.0.0", "nvm use 20.0.0"}, result.Commands)
}

func TestWithVersion_FallbackStrategy_DefaultVersionUsed(t *testing.T) {
	// Node fallback has DefaultVersion "24" — substituted when MinimumVersion is empty.
	fs, ok := selectInstallStrategy("node", "", map[string]bool{}).(FallbackStrategy)
	require.True(t, ok)

	result, ok := fs.withVersion("").(FallbackStrategy)

	require.True(t, ok)
	assert.Contains(t, result.Commands, "nvm install 24")
	assert.Contains(t, result.Commands, "nvm use 24")
}

func TestWithVersion_FallbackStrategy_MinimumVersionOverridesDefault(t *testing.T) {
	// When MinimumVersion is set, it takes precedence over DefaultVersion.
	fs, ok := selectInstallStrategy("node", "", map[string]bool{}).(FallbackStrategy)
	require.True(t, ok)

	result, ok := fs.withVersion("20.0.0").(FallbackStrategy)

	require.True(t, ok)
	assert.Contains(t, result.Commands, "nvm install 20.0.0")
	assert.Contains(t, result.Commands, "nvm use 20.0.0")
}
