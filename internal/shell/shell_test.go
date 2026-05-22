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

package shell

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParentProcessName_ReturnsNonEmpty(t *testing.T) {
	name := parentProcessName()

	// In the test runner the parent is always determinable on Linux/macOS
	// (either via /proc or ps). We don't assert a specific name because
	// it depends on the test runner (e.g. "go", "task", etc.).
	assert.NotEmpty(t, name)
}

func TestDetectShell_ReturnsNonEmpty(t *testing.T) {
	name, err := DetectShell()

	require.NoError(t, err)
	assert.NotEmpty(t, name)
}

func TestNormalizeShellName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{input: "pwsh", expected: "powershell"},
		{input: "powershell", expected: "powershell"},
		{input: "bash", expected: "bash"},
		{input: "zsh", expected: "zsh"},
		{input: "fish", expected: "fish"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, normalizeShellName(tt.input))
		})
	}
}

func TestDetectShell_EnvVarFallback(t *testing.T) {
	// Verify that $SHELL is used when set (simulates the env var fallback path).
	t.Setenv("SHELL", "/usr/bin/fish")

	// parentProcessName() will still return the test runner's parent, so we
	// can't easily force the $SHELL fallback in a unit test without process
	// manipulation. Instead, assert DetectShell returns a non-empty result.
	name, err := DetectShell()

	require.NoError(t, err)
	assert.NotEmpty(t, name)
}

// TestShellFromEnvPath exercises normalizeShellName(filepath.Base(shellPath)),
// which is the exact transformation DetectShell applies to the SHELL env var.
// Fixture values cover Unix absolute paths, macOS Homebrew paths, bare names
// (already resolved by the user's PATH), and PowerShell variants.
func TestShellFromEnvPath(t *testing.T) {
	tests := []struct {
		shellEnv string
		want     string
	}{
		// Standard Unix absolute paths
		{shellEnv: "/bin/bash", want: "bash"},
		{shellEnv: "/bin/sh", want: "sh"},
		{shellEnv: "/usr/bin/bash", want: "bash"},
		{shellEnv: "/usr/bin/zsh", want: "zsh"},
		{shellEnv: "/usr/bin/fish", want: "fish"},
		// macOS system paths
		{shellEnv: "/bin/zsh", want: "zsh"},
		// macOS Homebrew paths (Intel)
		{shellEnv: "/usr/local/bin/bash", want: "bash"},
		{shellEnv: "/usr/local/bin/zsh", want: "zsh"},
		{shellEnv: "/usr/local/bin/fish", want: "fish"},
		// macOS Homebrew paths (Apple Silicon)
		{shellEnv: "/opt/homebrew/bin/bash", want: "bash"},
		{shellEnv: "/opt/homebrew/bin/zsh", want: "zsh"},
		{shellEnv: "/opt/homebrew/bin/fish", want: "fish"},
		// Bare names — already resolved via PATH in the user's config
		{shellEnv: "bash", want: "bash"},
		{shellEnv: "zsh", want: "zsh"},
		{shellEnv: "fish", want: "fish"},
		{shellEnv: "sh", want: "sh"},
		// PowerShell variants — pwsh must normalise to "powershell"
		{shellEnv: "/usr/local/bin/pwsh", want: "powershell"},
		{shellEnv: "/opt/homebrew/bin/pwsh", want: "powershell"},
		{shellEnv: "pwsh", want: "powershell"},
		{shellEnv: "powershell", want: "powershell"},
	}

	for _, tt := range tests {
		t.Run(tt.shellEnv, func(t *testing.T) {
			got := normalizeShellName(filepath.Base(tt.shellEnv))

			assert.Equal(t, tt.want, got)
		})
	}
}

// TestDetectShell_ShellVersionEnvVars intentionally omitted: ZSH_VERSION,
// BASH_VERSION, and FISH_VERSION are shell-only parameters, not environment
// variables — they are never inherited by subprocesses and cannot be used for
// detection. The $SHELL env var is the correct fallback (see DetectShell).

func TestIsSupportedShell(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "bash", want: true},
		{name: "zsh", want: true},
		{name: "fish", want: true},
		{name: "powershell", want: true},
		{name: "ruby", want: false},
		{name: "python", want: false},
		{name: "sh", want: false},
		{name: "node", want: false},
		{name: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isSupportedShell(tt.name))
		})
	}
}

// TestDetectShell_SHELLEnvFixtures calls DetectShell with SHELL set to a
// variety of fixture values and verifies it always returns a usable, non-empty
// name. Because parentProcessName() succeeds in the test runner the env-var
// branch is not exercised here, but the table documents expected end-to-end
// behaviour and guards against regressions in DetectShell's return type.
func TestDetectShell_SHELLEnvFixtures(t *testing.T) {
	tests := []struct {
		shellEnv string
	}{
		{shellEnv: "/bin/bash"},
		{shellEnv: "/bin/sh"},
		{shellEnv: "/usr/bin/zsh"},
		{shellEnv: "/usr/local/bin/fish"},
		{shellEnv: "/usr/local/bin/pwsh"},
		{shellEnv: "/opt/homebrew/bin/zsh"},
		{shellEnv: "bash"},
		{shellEnv: "zsh"},
		{shellEnv: "fish"},
		{shellEnv: "pwsh"},
	}

	for _, tt := range tests {
		t.Run(tt.shellEnv, func(t *testing.T) {
			t.Setenv("SHELL", tt.shellEnv)

			name, err := DetectShell()

			require.NoError(t, err)
			assert.NotEmpty(t, name)
		})
	}
}

// TestSupportedShells verifies that the canonical shell list is complete and
// that every entry matches the normalised form DetectShell produces.
func TestSupportedShells(t *testing.T) {
	supported := SupportedShells()

	assert.Contains(t, supported, string(Bash))
	assert.Contains(t, supported, string(Zsh))
	assert.Contains(t, supported, string(Fish))
	assert.Contains(t, supported, string(PowerShell))
	assert.Len(t, supported, 4)
}

// TestResolveShell_SpecifiedShell verifies that an explicitly provided shell name
// is returned unchanged without invoking detection.
func TestResolveShell_SpecifiedShell(t *testing.T) {
	shells := []string{"bash", "zsh", "fish", "powershell"}

	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			got, err := ResolveShell(shell)

			require.NoError(t, err)
			assert.Equal(t, shell, got)
		})
	}
}

// TestResolveShell_AutoDetect verifies that an empty specifiedShell triggers
// shell detection and returns a non-empty result.
func TestResolveShell_AutoDetect(t *testing.T) {
	got, err := ResolveShell("")

	require.NoError(t, err)
	assert.NotEmpty(t, got)
}

// TestParentProcessNameWindows_InvalidPID verifies that an unreachable PID
// returns an empty string. On non-Windows systems tasklist is not available,
// so the exec will fail and the function must return "".
func TestParentProcessNameWindows_InvalidPID(t *testing.T) {
	got := parentProcessNameWindows(1 << 30)

	assert.Empty(t, got)
}

// TestNormalizeShellName_EdgeCases extends the baseline table with variants
// that could plausibly reach the normaliser (e.g. version-suffixed binaries,
// mixed case, empty input).
func TestNormalizeShellName_EdgeCases(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// Unknown shells are returned unchanged
		{input: "sh", want: "sh"},
		{input: "dash", want: "dash"},
		{input: "ksh", want: "ksh"},
		{input: "tcsh", want: "tcsh"},
		{input: "csh", want: "csh"},
		// Empty input round-trips as-is
		{input: "", want: ""},
		// Mixed-case is not normalised (OS provides canonical form)
		{input: "Bash", want: "Bash"},
		{input: "ZSH", want: "ZSH"},
		// Only "pwsh" maps to "powershell"; other PowerShell names are unchanged
		{input: "pwsh.exe", want: "pwsh.exe"},
		{input: "powershell.exe", want: "powershell.exe"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeShellName(tt.input))
		})
	}
}
