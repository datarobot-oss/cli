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

package tools

import (
	"runtime"
	"testing"

	"github.com/datarobot/cli/internal/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSufficientVersionTrue(t *testing.T) {
	sufficientCases := []struct{ installed, minimal string }{
		{installed: "3.5.7", minimal: "3.5.7"},
		{installed: "3.5.9", minimal: "3.5.7"},
		{installed: "3.7.6", minimal: "3.5.7"},
		{installed: "5.4.6", minimal: "3.5.7"},
		// Git-describe format with pre-release metadata
		{installed: "v0.2.55-beta.0-0-gabcd1234", minimal: "0.2.54"},
		{installed: "v0.2.55-beta.0-0-gabcd1234", minimal: "0.2.55"},
		// Fallback format for fresh clones
		{installed: "v0.0.0-dev.42.gabcd1234", minimal: "0.0.0"},
	}

	for _, testCase := range sufficientCases {
		if _, ok := sufficientVersion(testCase.installed, testCase.minimal); ok != true {
			t.Errorf("for installed %s and minimal %s, expected sufficient", testCase.installed, testCase.minimal)
		}
	}
}

func TestSufficientVersionFalse(t *testing.T) {
	sufficientCases := []struct{ installed, minimal string }{
		{installed: "2.6.8", minimal: "3.5.7"},
		{installed: "3.4.8", minimal: "3.5.7"},
		{installed: "3.5.6", minimal: "3.5.7"},
		// Git-describe format with pre-release metadata
		{installed: "v0.2.55-beta.0-0-gabcd1234", minimal: "0.2.56"},
		// Fallback format is insufficient for higher minimal
		{installed: "v0.0.0-dev.42.gabcd1234", minimal: "0.1.0"},
	}

	for _, testCase := range sufficientCases {
		if _, ok := sufficientVersion(testCase.installed, testCase.minimal); ok != false {
			t.Errorf("for installed %s and minimal %s, expected insufficient", testCase.installed, testCase.minimal)
		}
	}
}

func TestSufficientSelfVersion(t *testing.T) {
	tests := []struct {
		name           string
		versionValue   string
		minimalVersion string
		expected       bool
	}{
		{
			name:           "dev always returns true",
			versionValue:   "dev",
			minimalVersion: "99.99.99",
			expected:       true,
		},
		{
			name:           "git-describe format sufficient",
			versionValue:   "v0.2.55-beta.0-0-gabcd1234",
			minimalVersion: "0.2.54",
			expected:       true,
		},
		{
			name:           "git-describe format exactly matching",
			versionValue:   "v0.2.55-beta.0-0-gabcd1234",
			minimalVersion: "0.2.55",
			expected:       true,
		},
		{
			name:           "git-describe format insufficient",
			versionValue:   "v0.2.55-beta.0-0-gabcd1234",
			minimalVersion: "0.2.56",
			expected:       false,
		},
		{
			name:           "fallback dev format sufficient",
			versionValue:   "v0.0.0-dev.42.gabcd1234",
			minimalVersion: "0.0.0",
			expected:       true,
		},
		{
			name:           "empty minimal returns false",
			versionValue:   "v0.2.55-beta.0-0-gabcd1234",
			minimalVersion: "",
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalVersion := version.Version

			defer func() { version.Version = originalVersion }()

			version.Version = tt.versionValue
			result := SufficientSelfVersion(tt.minimalVersion)
			assert.Equal(t, tt.expected, result, "version=%s minimal=%s", tt.versionValue, tt.minimalVersion)
		})
	}
}

func TestPrerequisitesMsg_BothEmpty(t *testing.T) {
	out := PrerequisitesMsg(nil, nil)

	assert.Equal(t, "\n", out)
}

func TestPrerequisitesMsg_MissingOnly(t *testing.T) {
	out := PrerequisitesMsg([]string{"uv 0.4.0 (https://example.com)"}, nil)

	assert.Contains(t, out, "Missing required tools")
	assert.Contains(t, out, "uv 0.4.0")
	assert.NotContains(t, out, "Wrong versions")
}

func TestPrerequisitesMsg_WrongVersionOnly(t *testing.T) {
	out := PrerequisitesMsg(nil, []string{"task (minimal: v3.35.0, installed: v3.32.0)"})

	assert.Contains(t, out, "Wrong versions of tools")
	assert.Contains(t, out, "task (minimal: v3.35.0")
	assert.NotContains(t, out, "Missing required")
}

func TestPrerequisitesMsg_Both(t *testing.T) {
	out := PrerequisitesMsg(
		[]string{"uv 0.4.0 (https://example.com)"},
		[]string{"task (minimal: v3.35.0, installed: v3.32.0)"},
	)

	assert.Contains(t, out, "Missing required tools")
	assert.Contains(t, out, "uv 0.4.0")
	assert.Contains(t, out, "Wrong versions of tools")
	assert.Contains(t, out, "task (minimal: v3.35.0")
}

func TestPrerequisitesMsg_MultipleEntries(t *testing.T) {
	out := PrerequisitesMsg([]string{"uv", "pulumi"}, []string{"task"})

	assert.Contains(t, out, "\t- uv")
	assert.Contains(t, out, "\t- pulumi")
	assert.Contains(t, out, "\t- task")
}

func TestPrerequisitesMsg_EndsWithNewline(t *testing.T) {
	out := PrerequisitesMsg([]string{"uv"}, nil)

	assert.True(t, len(out) > 0 && out[len(out)-1] == '\n')
}

func TestCheckPrerequisites_AllSatisfied(t *testing.T) {
	orig := RequiredTools

	defer func() { RequiredTools = orig }()

	// "sh" is always present and has no version constraint.
	RequiredTools = []Prerequisite{
		{Name: "sh", Command: "sh"},
	}

	missing, wrongVer, missingMsgs, wrongVerMsgs := CheckPrerequisites()

	assert.Empty(t, missing)
	assert.Empty(t, wrongVer)
	assert.Empty(t, missingMsgs)
	assert.Empty(t, wrongVerMsgs)
}

func TestCheckPrerequisites_MissingTool(t *testing.T) {
	orig := RequiredTools

	defer func() { RequiredTools = orig }()

	RequiredTools = []Prerequisite{
		{Name: "FakeTool", Command: "nonexistent_dr_fake_tool_xyz", URL: "https://example.com"},
	}

	missing, wrongVer, missingMsgs, wrongVerMsgs := CheckPrerequisites()

	require.Len(t, missing, 1)
	assert.Equal(t, "FakeTool", missing[0].Name)
	assert.Empty(t, wrongVer)
	assert.Len(t, missingMsgs, 1)
	assert.Contains(t, missingMsgs[0], "FakeTool")
	assert.Contains(t, missingMsgs[0], "https://example.com")
	assert.Empty(t, wrongVerMsgs)
}

func TestCheckPrerequisites_WrongVersion(t *testing.T) {
	orig := RequiredTools

	defer func() { RequiredTools = orig }()

	// "echo 1.0.0" is always installed; its output "1.0.0" is insufficient vs "2.0.0".
	RequiredTools = []Prerequisite{
		{Name: "Echo", Command: "echo 1.0.0", MinimumVersion: "2.0.0", URL: "https://example.com"},
	}

	missing, wrongVer, missingMsgs, wrongVerMsgs := CheckPrerequisites()

	assert.Empty(t, missing)
	assert.Empty(t, missingMsgs)
	require.Len(t, wrongVer, 1)
	assert.Equal(t, "Echo", wrongVer[0].Name)
	assert.Len(t, wrongVerMsgs, 1)
	assert.Contains(t, wrongVerMsgs[0], "Echo")
}

func TestCheckPrerequisites_Mixed(t *testing.T) {
	orig := RequiredTools

	defer func() { RequiredTools = orig }()

	RequiredTools = []Prerequisite{
		{Name: "sh", Command: "sh"},
		{Name: "FakeTool", Command: "nonexistent_dr_fake_tool_xyz"},
		{Name: "Echo", Command: "echo 1.0.0", MinimumVersion: "2.0.0"},
	}

	missing, wrongVer, missingMsgs, wrongVerMsgs := CheckPrerequisites()

	require.Len(t, missing, 1)
	assert.Equal(t, "FakeTool", missing[0].Name)
	require.Len(t, wrongVer, 1)
	assert.Equal(t, "Echo", wrongVer[0].Name)
	assert.Len(t, missingMsgs, 1)
	assert.Len(t, wrongVerMsgs, 1)
}

func TestPlatformInstallCommand_CurrentPlatform(t *testing.T) {
	p := Prerequisite{
		Name: "uv",
		Install: InstallCommands{
			MacOS:   "brew install uv",
			Linux:   "curl -Ls https://astral.sh/uv/install.sh | sh",
			Windows: "powershell install uv",
		},
	}

	var expected string

	switch runtime.GOOS {
	case "darwin":
		expected = p.Install.MacOS
	case "linux":
		expected = p.Install.Linux
	case "windows":
		expected = p.Install.Windows
	default:
		t.Skipf("unsupported platform %q", runtime.GOOS)
	}

	cmd, err := p.PlatformInstallCommand()

	require.NoError(t, err)
	assert.Equal(t, expected, cmd)
}

func TestPlatformInstallCommand_EmptyCommandReturnsError(t *testing.T) {
	p := Prerequisite{Name: "uv"} // all install commands are zero-value empty strings

	_, err := p.PlatformInstallCommand()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "uv")
}
