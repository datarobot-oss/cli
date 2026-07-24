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

package registry

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"
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

// buildEnv runs manager detection with injectable dependencies (test helper).
func buildEnv(lookPath func(string) (string, error), getenv func(string) string, dirExists func(string) bool, goos string) map[string]bool {
	ctx := detectionCtx{lookPath: lookPath, getenv: getenv, dirExists: dirExists, goos: goos}

	env := make(map[string]bool, len(knownManagers))

	for _, m := range knownManagers {
		env[m.Name] = m.present(ctx)
	}

	return env
}

// ──────────────────────────────────────────────────────────────
// initToolNameMap
// ──────────────────────────────────────────────────────────────

func TestBuildToolNameMap_RegistryKeysMapToThemselves(t *testing.T) {
	toolNameMap := buildToolNameMap()

	for key := range ToolRegistry {
		assert.Equal(t, key, toolNameMap[strings.ToLower(key)],
			"registry key %q should be reachable via its lowercase form", key)
	}
}

func TestBuildToolNameMap_NameLowercasedIsRegistered(t *testing.T) {
	toolNameMap := buildToolNameMap()

	for key, info := range ToolRegistry {
		lower := strings.ToLower(info.Name)

		assert.Equal(t, key, toolNameMap[lower],
			"lowercased Name %q should resolve to key %q", lower, key)
	}
}

func TestBuildToolNameMap_AliasesAreRegistered(t *testing.T) {
	toolNameMap := buildToolNameMap()

	for key, info := range ToolRegistry {
		for _, alias := range info.Aliases {
			assert.Equal(t, key, toolNameMap[strings.ToLower(alias)],
				"alias %q should resolve to key %q", alias, key)
		}
	}
}

func TestBuildToolNameMap_ProducesNonEmptyMap(t *testing.T) {
	assert.NotEmpty(t, buildToolNameMap())
}

func TestBuildToolNameMap_UnknownNameNotPresent(t *testing.T) {
	toolNameMap := buildToolNameMap()

	_, ok := toolNameMap["this-tool-does-not-exist"]

	assert.False(t, ok)
}

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
	env := buildEnv(fakeLookPath("brew"), fakeGetenv(nil), noDirExists, "darwin")

	assert.True(t, env["brew"])
	assert.False(t, env["winget"])
}

func TestDetectEnvironment_BrewSuppressedOnWindows(t *testing.T) {
	// Even if brew is on PATH, it must not be treated as available on Windows.
	env := buildEnv(fakeLookPath("brew", "winget", "choco"), fakeGetenv(nil), noDirExists, "windows")

	assert.False(t, env["brew"])
	assert.True(t, env["winget"])
	assert.True(t, env["choco"])
}

func TestDetectEnvironment_PyenvDetected(t *testing.T) {
	env := buildEnv(fakeLookPath("pyenv"), fakeGetenv(nil), noDirExists, "linux")

	assert.True(t, env["pyenv"])
	assert.False(t, env["brew"])
}

func TestDetectEnvironment_NVM_ViaEnvVar(t *testing.T) {
	getenv := fakeGetenv(map[string]string{"NVM_DIR": "/home/user/.nvm"})
	dirExists := func(p string) bool { return p == "/home/user/.nvm" }

	env := buildEnv(fakeLookPath(), getenv, dirExists, "linux")

	assert.True(t, env["nvm"])
}

func TestDetectEnvironment_NVM_ViaHomeFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on os.UserHomeDir honoring $HOME, which is POSIX-only")
	}

	t.Setenv("HOME", "/home/user")

	dirExists := func(p string) bool { return p == "/home/user/.nvm" }

	env := buildEnv(fakeLookPath(), fakeGetenv(nil), dirExists, "linux")

	assert.True(t, env["nvm"])
}

func TestDetectEnvironment_NVM_HomeDirError_NVMAbsent(t *testing.T) {
	// Unset HOME so os.UserHomeDir() cannot resolve a home directory;
	// nvm must be reported absent rather than panicking.
	t.Setenv("HOME", "")

	env := buildEnv(fakeLookPath(), fakeGetenv(nil), noDirExists, "linux")

	assert.False(t, env["nvm"])
}

func TestDetectEnvironment_NVM_AbsentOnWindows(t *testing.T) {
	getenv := fakeGetenv(map[string]string{"NVM_DIR": "/home/user/.nvm"})
	dirExists := func(p string) bool { return true }

	// nvm must never be reported on Windows even if the directory exists.
	env := buildEnv(fakeLookPath(), getenv, dirExists, "windows")

	assert.False(t, env["nvm"])
}

func TestDetectEnvironment_AllAbsent(t *testing.T) {
	env := buildEnv(fakeLookPath(), fakeGetenv(nil), noDirExists, "linux")

	for key, val := range env {
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

	assert.Equal(t, "  Tip: You have pyenv — try: pip install uv", ms.GetStrategyTip("linux"))
}

func TestManagerStrategy_GetStrategyTip_MultipleCommands(t *testing.T) {
	ms := ManagerStrategy{
		Manager:  "asdf",
		Commands: []string{"asdf install uv latest", "asdf global uv latest"},
	}

	assert.Equal(t, TAB+"Tip: You have asdf — try: \n"+TAB+TAB+"asdf install uv latest\n"+TAB+TAB+"asdf global uv latest", ms.GetStrategyTip("linux"))
}

func TestManagerStrategy_GetStrategyTip_GoosIgnored(t *testing.T) {
	ms := ManagerStrategy{Manager: "brew", Commands: []string{"brew install uv"}}

	assert.Equal(t, ms.GetStrategyTip("linux"), ms.GetStrategyTip("windows"), "goos must not affect ManagerStrategy tip")
}

// ──────────────────────────────────────────────────────────────
// getStrategyTip — FallbackStrategy
// ──────────────────────────────────────────────────────────────

func TestFallbackStrategy_GetStrategyTip_SingleCommand(t *testing.T) {
	fs := FallbackStrategy{Commands: []string{"curl -LsSf https://astral.sh/uv/install.sh | sh"}}

	assert.Equal(t, "  Try: curl -LsSf https://astral.sh/uv/install.sh | sh", fs.GetStrategyTip("linux"))
}

func TestFallbackStrategy_GetStrategyTip_MultipleCommands(t *testing.T) {
	fs := FallbackStrategy{Commands: []string{"curl https://pyenv.run | bash", "pyenv install 3.12", "pyenv global 3.12"}}

	assert.Equal(t, TAB+"Try:\n"+TAB+TAB+"curl https://pyenv.run | bash\n"+TAB+TAB+"pyenv install 3.12\n"+TAB+TAB+"pyenv global 3.12", fs.GetStrategyTip("linux"))
}

func TestFallbackStrategy_GetStrategyTip_URLOnly(t *testing.T) {
	fs := FallbackStrategy{URL: "https://git-scm.com/downloads"}

	assert.Equal(t, "  See: https://git-scm.com/downloads", fs.GetStrategyTip("linux"))
}

func TestFallbackStrategy_GetStrategyTip_Empty(t *testing.T) {
	assert.Empty(t, FallbackStrategy{}.GetStrategyTip("linux"))
}

func TestFallbackStrategy_GetStrategyTip_WindowsOverride(t *testing.T) {
	fs := FallbackStrategy{
		Commands:        []string{"curl -LsSf https://astral.sh/uv/install.sh | sh"},
		CommandsWindows: []string{`powershell -c "irm https://astral.sh/uv/install.ps1 | iex"`},
	}

	assert.Contains(t, fs.GetStrategyTip("windows"), "iex")
	assert.Contains(t, fs.GetStrategyTip("linux"), "curl")
}

// ──────────────────────────────────────────────────────────────
// selectInstallStrategy
// ──────────────────────────────────────────────────────────────

// TestSelectInstallStrategy_PyenvPresentBrewAbsent_UV is the acceptance-criteria test:
// with pyenv on PATH and brew absent, SelectInstallStrategy("uv") must return the
// pyenv ManagerStrategy (pip install uv).
func TestSelectInstallStrategy_PyenvPresentBrewAbsent_UV(t *testing.T) {
	env := map[string]bool{
		"pyenv": true,
		"brew":  false,
	}

	ms, ok := SelectInstallStrategy("uv", "", env).(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, "pyenv", ms.Manager)
	assert.Equal(t, []string{"pip install uv"}, ms.Commands)
}

func TestSelectInstallStrategy_BrewPresent_UV(t *testing.T) {
	env := map[string]bool{"brew": true}

	ms, ok := SelectInstallStrategy("uv", "", env).(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, []string{"brew install uv"}, ms.Commands)
}

func TestSelectInstallStrategy_BrewPresent_Python(t *testing.T) {
	env := map[string]bool{"brew": true}

	ms, ok := SelectInstallStrategy("python", "", env).(ManagerStrategy)

	require.True(t, ok)
	// Without version the placeholder is returned as-is; callers call .WithVersion().
	assert.Equal(t, []string{"brew install python@{version_mm}"}, ms.Commands)
}

func TestSelectInstallStrategy_FallbackUnix_WhenNoManagerDetected(t *testing.T) {
	env := map[string]bool{}

	fs, ok := SelectInstallStrategy("uv", "", env).(FallbackStrategy)

	require.True(t, ok)
	require.NotEmpty(t, fs.Commands)
	assert.Contains(t, fs.Commands[0], "curl")
}

func TestSelectInstallStrategy_FallbackWindows(t *testing.T) {
	env := map[string]bool{"is_windows": true}

	fs, ok := SelectInstallStrategy("uv", "", env).(FallbackStrategy)

	require.True(t, ok)
	require.NotEmpty(t, fs.CommandsWindows)
	assert.Contains(t, fs.CommandsWindows[0], "iex")
}

func TestSelectInstallStrategy_UnknownTool(t *testing.T) {
	env := map[string]bool{"brew": true}

	result := SelectInstallStrategy("nonexistent-tool", "", env)

	assert.Nil(t, result)
}

func TestSelectInstallStrategy_WingetPresent_Task(t *testing.T) {
	env := map[string]bool{"winget": true, "is_windows": true}

	ms, ok := SelectInstallStrategy("task", "", env).(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, []string{"winget install Task.Task"}, ms.Commands)
}

func TestSelectInstallStrategy_NVMPresent_Node(t *testing.T) {
	env := map[string]bool{"nvm": true}

	ms, ok := SelectInstallStrategy("node", "", env).(ManagerStrategy)

	require.True(t, ok)
	// Without version the placeholder is returned as-is; callers call .WithVersion().
	assert.Equal(t, []string{"nvm install {version}", "nvm use {version}"}, ms.Commands)
}

func TestSelectInstallStrategy_AllToolsHaveAtLeastOneFallback(t *testing.T) {
	emptyEnv := map[string]bool{}

	for key := range ToolRegistry {
		t.Run(key, func(t *testing.T) {
			result := SelectInstallStrategy(key, "", emptyEnv)
			assert.NotNil(t, result, "tool %q must have a fallback strategy", key)
		})
	}
}

// ──────────────────────────────────────────────────────────────
// ToolRegistry structural rules
// ──────────────────────────────────────────────────────────────

func TestToolRegistry_KeysAreLowercase(t *testing.T) {
	for key := range ToolRegistry {
		assert.Equal(t, strings.ToLower(key), key, "registry key %q must be lowercase", key)
	}
}

func TestToolRegistry_LastStrategyIsFallback(t *testing.T) {
	for key, info := range ToolRegistry {
		require.NotEmpty(t, info.Strategies, "tool %q has no strategies", key)

		last := info.Strategies[len(info.Strategies)-1]

		_, ok := last.(FallbackStrategy)

		assert.True(t, ok, "tool %q: last strategy must be FallbackStrategy, got %T", key, last)
	}
}

func TestToolRegistry_ManagerStrategiesReferenceKnownManagers(t *testing.T) {
	known := make(map[string]bool, len(knownManagers))

	for _, m := range knownManagers {
		known[m.Name] = true
	}

	for key, info := range ToolRegistry {
		for i, s := range info.Strategies {
			ms, ok := s.(ManagerStrategy)

			if !ok {
				continue
			}

			assert.True(t, known[ms.Manager],
				"tool %q ManagerStrategy[%d]: manager %q is not in knownManagers", key, i, ms.Manager)
		}
	}
}

func TestSelectInstallStrategy_SkipsFailedMgr(t *testing.T) {
	env := map[string]bool{"pyenv": true, "brew": true}

	ms, ok := SelectInstallStrategy("uv", "pyenv", env).(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, "brew", ms.Manager, "should skip pyenv and return brew")
}

// ──────────────────────────────────────────────────────────────
// withVersion
// ──────────────────────────────────────────────────────────────

func TestWithVersion_ManagerStrategy_Pyenv_Python(t *testing.T) {
	env := map[string]bool{"pyenv": true}

	ms, ok := SelectInstallStrategy("python", "", env).(ManagerStrategy)
	require.True(t, ok)

	result, ok := ms.WithVersion("3.9.6").(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, []string{"pyenv install 3.9.6", "pyenv global 3.9.6"}, result.Commands)
}

func TestWithVersion_ManagerStrategy_Asdf_Python(t *testing.T) {
	env := map[string]bool{"asdf": true}

	ms, ok := SelectInstallStrategy("python", "", env).(ManagerStrategy)
	require.True(t, ok)

	result, ok := ms.WithVersion("3.9.6").(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, []string{"asdf install python 3.9.6", "asdf global python 3.9.6"}, result.Commands)
}

func TestWithVersion_ManagerStrategy_Brew_Python(t *testing.T) {
	env := map[string]bool{"brew": true}

	ms, ok := SelectInstallStrategy("python", "", env).(ManagerStrategy)
	require.True(t, ok)

	result, ok := ms.WithVersion("3.9.6").(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, []string{"brew install python@3.9"}, result.Commands)
}

func TestWithVersion_ManagerStrategy_EmptyVersion_UsesDefault(t *testing.T) {
	// python/pyenv has DefaultVersion "3.14" — used when MinimumVersion is empty.
	env := map[string]bool{"pyenv": true}

	ms, ok := SelectInstallStrategy("python", "", env).(ManagerStrategy)
	require.True(t, ok)

	result, ok := ms.WithVersion("").(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, []string{"pyenv install 3.14", "pyenv global 3.14"}, result.Commands)
}

func TestWithVersion_FallbackStrategy_Python(t *testing.T) {
	fs, ok := SelectInstallStrategy("python", "", map[string]bool{}).(FallbackStrategy)
	require.True(t, ok)

	result, ok := fs.WithVersion("3.9.6").(FallbackStrategy)

	require.True(t, ok)
	assert.Contains(t, result.Commands, "pyenv install 3.9.6")
	assert.Contains(t, result.Commands, "pyenv global 3.9.6")
}

func TestWithVersion_FallbackStrategy_WindowsCommandsAlsoSubstituted(t *testing.T) {
	fs, ok := SelectInstallStrategy("python", "", map[string]bool{}).(FallbackStrategy)
	require.True(t, ok)

	result, ok := fs.WithVersion("3.9.6").(FallbackStrategy)

	require.True(t, ok)
	assert.Contains(t, result.CommandsWindows, "pyenv install 3.9.6")
	assert.Contains(t, result.CommandsWindows, "pyenv global 3.9.6")
}

func TestWithVersion_FallbackStrategy_URLOnlyUnchanged(t *testing.T) {
	// git has a URL-only FallbackStrategy — withVersion must not panic and must
	// leave the URL intact.
	fs, ok := SelectInstallStrategy("git", "", map[string]bool{}).(FallbackStrategy)
	require.True(t, ok)

	result, ok := fs.WithVersion("2.40.0").(FallbackStrategy)

	require.True(t, ok)
	assert.Equal(t, "https://git-scm.com/downloads", result.URL)
	assert.Empty(t, result.Commands)
}

func TestWithVersion_ManagerStrategy_NoPlaceholders(t *testing.T) {
	// uv's brew strategy has no version placeholders — commands must be returned as-is.
	env := map[string]bool{"brew": true}

	ms, ok := SelectInstallStrategy("uv", "", env).(ManagerStrategy)
	require.True(t, ok)

	result, ok := ms.WithVersion("0.11.20").(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, []string{"brew install uv"}, result.Commands)
}

func TestWithVersion_ManagerStrategy_EmptyVersion_NoDefaultVersion_NoPlaceholders(t *testing.T) {
	// brew/node has no DefaultVersion and no {version} placeholder — commands pass
	// through substituteCmds unchanged when both version and DefaultVersion are empty.
	env := map[string]bool{"brew": true}

	ms, ok := SelectInstallStrategy("node", "", env).(ManagerStrategy)
	require.True(t, ok)

	result, ok := ms.WithVersion("").(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, []string{"brew install node"}, result.Commands)
}

func TestSelectInstallStrategy_DisplayNameNormalized(t *testing.T) {
	// selectInstallStrategy calls NormalizeToolName internally, so display names
	// like "Python" (capital P) must resolve to the "python" registry entry.
	env := map[string]bool{"brew": true}

	ms, ok := SelectInstallStrategy("Python", "", env).(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, "brew", ms.Manager)
}

func TestSelectInstallStrategy_FailedMgrIsOnlyDetectedMgr_FallsToFallback(t *testing.T) {
	// brew is the only detected manager but it already failed — must skip it and
	// return the FallbackStrategy rather than nil.
	env := map[string]bool{"brew": true}

	_, ok := SelectInstallStrategy("uv", "brew", env).(FallbackStrategy)

	require.True(t, ok)
}

func TestWithVersion_ManagerStrategy_DefaultVersion_UsedWhenVersionEmpty(t *testing.T) {
	// nvm strategy for node has DefaultVersion "24" — used when MinimumVersion is empty.
	env := map[string]bool{"nvm": true}

	ms, ok := SelectInstallStrategy("node", "", env).(ManagerStrategy)
	require.True(t, ok)

	result, ok := ms.WithVersion("").(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, []string{"nvm install 24", "nvm use 24"}, result.Commands)
}

func TestWithVersion_ManagerStrategy_MinimumVersionOverridesDefault(t *testing.T) {
	// When MinimumVersion is set, it takes precedence over DefaultVersion.
	env := map[string]bool{"nvm": true}

	ms, ok := SelectInstallStrategy("node", "", env).(ManagerStrategy)
	require.True(t, ok)

	result, ok := ms.WithVersion("20.0.0").(ManagerStrategy)

	require.True(t, ok)
	assert.Equal(t, []string{"nvm install 20.0.0", "nvm use 20.0.0"}, result.Commands)
}

func TestWithVersion_FallbackStrategy_DefaultVersionUsed(t *testing.T) {
	// Node fallback has DefaultVersion "24" — substituted when MinimumVersion is empty.
	fs, ok := SelectInstallStrategy("node", "", map[string]bool{}).(FallbackStrategy)
	require.True(t, ok)

	result, ok := fs.WithVersion("").(FallbackStrategy)

	require.True(t, ok)
	assert.Contains(t, result.Commands, "nvm install 24")
	assert.Contains(t, result.Commands, "nvm use 24")
}

func TestWithVersion_FallbackStrategy_MinimumVersionOverridesDefault(t *testing.T) {
	// When MinimumVersion is set, it takes precedence over DefaultVersion.
	fs, ok := SelectInstallStrategy("node", "", map[string]bool{}).(FallbackStrategy)
	require.True(t, ok)

	result, ok := fs.WithVersion("20.0.0").(FallbackStrategy)

	require.True(t, ok)
	assert.Contains(t, result.Commands, "nvm install 20.0.0")
	assert.Contains(t, result.Commands, "nvm use 20.0.0")
}

// ──────────────────────────────────────────────────────────────
// DefaultVersion completeness
// ──────────────────────────────────────────────────────────────

func hasVersionPlaceholder(cmds []string) bool {
	for _, c := range cmds {
		if strings.Contains(c, "{version}") || strings.Contains(c, "{version_mm}") {
			return true
		}
	}

	return false
}

func TestToolRegistry_VersionPlaceholderRequiresDefaultVersion(t *testing.T) {
	for toolKey, info := range ToolRegistry {
		for i, s := range info.Strategies {
			switch strategy := s.(type) {
			case ManagerStrategy:
				if hasVersionPlaceholder(strategy.Commands) {
					assert.NotEmpty(t, strategy.DefaultVersion,
						"tool %q ManagerStrategy[%d] (manager=%q) has {version} placeholder but no DefaultVersion",
						toolKey, i, strategy.Manager)
				}

			case FallbackStrategy:
				if hasVersionPlaceholder(strategy.Commands) || hasVersionPlaceholder(strategy.CommandsWindows) {
					assert.NotEmpty(t, strategy.DefaultVersion,
						"tool %q FallbackStrategy[%d] has {version} placeholder but no DefaultVersion",
						toolKey, i)
				}
			}
		}
	}
}

var versionPattern = regexp.MustCompile(`^\d+(\.\d+)*$`)

func TestToolRegistry_DefaultVersionIsValidVersion(t *testing.T) {
	for toolKey, info := range ToolRegistry {
		for i, s := range info.Strategies {
			switch strategy := s.(type) {
			case ManagerStrategy:
				if strategy.DefaultVersion != "" {
					assert.True(t, versionPattern.MatchString(strategy.DefaultVersion),
						"tool %q ManagerStrategy[%d] (manager=%q) DefaultVersion %q is not a valid version",
						toolKey, i, strategy.Manager, strategy.DefaultVersion)
				}

			case FallbackStrategy:
				if strategy.DefaultVersion != "" {
					assert.True(t, versionPattern.MatchString(strategy.DefaultVersion),
						"tool %q FallbackStrategy[%d] DefaultVersion %q is not a valid version",
						toolKey, i, strategy.DefaultVersion)
				}
			}
		}
	}
}
