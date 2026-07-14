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

package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePluginName(t *testing.T) {
	valid := []string{
		"assist",
		"my-plugin",
		"plugin_v2",
		"dr-apps",
	}

	for _, name := range valid {
		t.Run("valid/"+name, func(t *testing.T) {
			assert.NoError(t, validatePluginName(name))
		})
	}

	invalid := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"dotdot", ".."},
		{"dot", "."},
		{"unix traversal", "../../etc/passwd"},
		{"unix separator", "foo/bar"},
		{"windows separator", `foo\bar`},
		{"leading slash", "/absolute"},
	}

	for _, tt := range invalid {
		t.Run("invalid/"+tt.name, func(t *testing.T) {
			err := validatePluginName(tt.input)

			require.Error(t, err)
		})
	}
}

func TestValidateManifests_Matching(t *testing.T) {
	manifest := PluginManifest{
		BasicPluginManifest: BasicPluginManifest{
			Name:           "test",
			Version:        "1.0.0",
			Description:    "Test plugin",
			Authentication: true,
		},
		MinCLIVersion: "1.0.0",
	}

	err := validateManifests(manifest, manifest)

	assert.NoError(t, err)
}

func TestValidateManifests_Mismatch(t *testing.T) {
	expected := PluginManifest{
		BasicPluginManifest: BasicPluginManifest{
			Name:           "test",
			Version:        "1.0.0",
			Description:    "Test plugin",
			Authentication: true,
		},
		MinCLIVersion: "1.0.0",
	}

	actual := PluginManifest{
		BasicPluginManifest: BasicPluginManifest{
			Name:           "test",
			Version:        "2.0.0",
			Description:    "Different description",
			Authentication: false,
		},
		MinCLIVersion: "1.0.0",
	}

	err := validateManifests(expected, actual)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin script output does not match manifest.json")
	assert.Contains(t, err.Error(), "Version")
	assert.Contains(t, err.Error(), "Description")
	assert.Contains(t, err.Error(), "Authentication")
}

func TestValidateManifests_ManagedPlugin(t *testing.T) {
	manifest := PluginManifest{
		BasicPluginManifest: BasicPluginManifest{
			Name:           "apps",
			Version:        "11.1.0",
			Description:    "Host custom applications in DataRobot",
			Authentication: true,
		},
		Scripts: &PluginScripts{
			Posix:   "dr-apps.sh",
			Windows: "dr-apps.ps1",
		},
		MinCLIVersion: "0.2.0",
	}

	err := validateManifests(manifest, manifest)

	assert.NoError(t, err)
}

func TestValidateManifests_MissingScripts(t *testing.T) {
	// Scripts and MinCLIVersion are now ignored in validation
	// This allows managed plugins to work as PATH plugins
	expected := PluginManifest{
		BasicPluginManifest: BasicPluginManifest{
			Name:           "apps",
			Version:        "11.1.0",
			Description:    "Host custom applications in DataRobot",
			Authentication: true,
		},
		Scripts: &PluginScripts{
			Posix:   "dr-apps.sh",
			Windows: "dr-apps.ps1",
		},
		MinCLIVersion: "0.2.0",
	}

	actual := PluginManifest{
		BasicPluginManifest: BasicPluginManifest{
			Name:           "apps",
			Version:        "11.1.0",
			Description:    "Host custom applications in DataRobot",
			Authentication: true,
		},
		Scripts:       nil, // Different but ignored
		MinCLIVersion: "",  // Different but ignored
	}

	err := validateManifests(expected, actual)

	assert.NoError(t, err, "Scripts and MinCLIVersion differences should be ignored")
}

func TestValidateManifests_AllowExtraFields(t *testing.T) {
	// Simple manifest expects only basic fields
	expected := PluginManifest{
		BasicPluginManifest: BasicPluginManifest{
			Name:           "apps",
			Version:        "11.1.0",
			Description:    "Host custom applications",
			Authentication: true,
		},
	}

	// Actual output has additional managed plugin fields
	actual := PluginManifest{
		BasicPluginManifest: BasicPluginManifest{
			Name:           "apps",
			Version:        "11.1.0",
			Description:    "Host custom applications",
			Authentication: true,
		},
		Scripts: &PluginScripts{
			Posix:   "dr-apps.sh",
			Windows: "dr-apps.ps1",
		},
		MinCLIVersion: "0.2.0",
	}

	// Should pass - actual can have more fields than expected
	err := validateManifests(expected, actual)

	assert.NoError(t, err)
}

func TestFindPluginScript_NoExtension(t *testing.T) {
	tempDir := t.TempDir()

	scriptPath := filepath.Join(tempDir, "dr-test")
	createScript(t, scriptPath, "")

	found, err := FindPluginScript(tempDir, "test")

	require.NoError(t, err)
	assert.Equal(t, scriptPath, found)
}

func TestFindPluginScript_ShellExtension(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Shell script test only valid on Unix")
	}

	tempDir := t.TempDir()

	scriptPath := filepath.Join(tempDir, "dr-test.sh")
	createScript(t, scriptPath, "")

	found, err := FindPluginScript(tempDir, "test")

	require.NoError(t, err)
	assert.Equal(t, scriptPath, found)
}

func TestFindPluginScript_PowerShellExtension(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell script test only valid on Windows")
	}

	tempDir := t.TempDir()

	scriptPath := filepath.Join(tempDir, "dr-test.ps1")
	createScript(t, scriptPath, "")

	found, err := FindPluginScript(tempDir, "test")

	require.NoError(t, err)
	assert.Equal(t, scriptPath, found)
}

func TestFindPluginScript_NotFound(t *testing.T) {
	tempDir := t.TempDir()

	_, err := FindPluginScript(tempDir, "nonexistent")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin script not found")
	assert.Contains(t, err.Error(), "dr-nonexistent")
}

func TestFindPluginScript_EmptyDirectory(t *testing.T) {
	tempDir := t.TempDir()

	_, err := FindPluginScript(tempDir, "test")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "directory is empty")
}

func TestFindPluginScript_WrongExtensionIgnored(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix extension test only valid on non-Windows")
	}

	tempDir := t.TempDir()

	// Create a .ps1 file on Unix - should be ignored
	ps1Path := filepath.Join(tempDir, "dr-test.ps1")
	createScript(t, ps1Path, "")

	_, err := FindPluginScript(tempDir, "test")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin script not found")
}

func TestExecPluginManifest(t *testing.T) {
	tempDir := t.TempDir()

	expected := PluginManifest{
		BasicPluginManifest: BasicPluginManifest{
			Name:           "test",
			Version:        "1.0.0",
			Description:    "Test plugin",
			Authentication: true,
		},
		MinCLIVersion: "1.0.0",
	}

	data, err := json.Marshal(expected)
	require.NoError(t, err)

	scriptPath := writePluginManifestScript(t, tempDir, "dr-test", string(data))

	result, err := ExecPluginManifest(scriptPath)

	require.NoError(t, err)
	assert.Equal(t, expected, *result)
}

func TestExecPluginManifest_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()

	scriptPath := writePluginManifestScript(t, tempDir, "dr-test", "invalid json")

	_, err := ExecPluginManifest(scriptPath)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid manifest JSON")
}

func TestExecPluginManifest_ExtraField(t *testing.T) {
	tempDir := t.TempDir()

	// Script outputs extra field "authenticated" (typo)
	invalidJSON := `{"name":"test","version":"1.0.0","description":"Test plugin","authentication":false,"authenticated":true}`

	scriptPath := writePluginManifestScript(t, tempDir, "dr-test", invalidJSON)

	_, err := ExecPluginManifest(scriptPath)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid manifest JSON")
	assert.Contains(t, err.Error(), "authenticated")
}

// Helper functions

// writePluginManifestScript creates a cross-platform executable that responds to
// --dr-plugin-manifest by echoing manifestJSON. Returns the script path.
func writePluginManifestScript(t *testing.T, dir, name, manifestJSON string) string {
	t.Helper()

	var scriptPath, scriptContent string

	if runtime.GOOS == "windows" {
		scriptPath = filepath.Join(dir, name+".ps1")
		scriptContent = "#!/usr/bin/env pwsh\n" +
			"if ($args[0] -eq '--dr-plugin-manifest') {\n" +
			"  Write-Output '" + manifestJSON + "'\n" +
			"}\n"
	} else {
		scriptPath = filepath.Join(dir, name)
		scriptContent = "#!/bin/sh\n" +
			"if [ \"$1\" = \"--dr-plugin-manifest\" ]; then\n" +
			"  echo '" + manifestJSON + "'\n" +
			"else\n" +
			"  exit 0\n" +
			"fi\n"
	}

	createScript(t, scriptPath, scriptContent)

	return scriptPath
}

// writeExitScript creates a Unix shell script that exits with the given code.
// Returns the script path.
func writeExitScript(t *testing.T, dir, name string, exitCode int) string {
	t.Helper()

	script := fmt.Sprintf("#!/bin/sh\nexit %d\n", exitCode)
	path := filepath.Join(dir, name)

	createScript(t, path, script)

	return path
}

func createScript(t *testing.T, path, content string) {
	t.Helper()

	err := os.WriteFile(path, []byte(content), 0o755)
	require.NoError(t, err)
}

func TestValidateLicense_Success(t *testing.T) {
	tempDir := t.TempDir()

	licensePath := filepath.Join(tempDir, "LICENSE.txt")
	err := os.WriteFile(licensePath, []byte("Apache License 2.0"), 0o644)
	require.NoError(t, err)

	err = ValidateLicense(tempDir)

	assert.NoError(t, err)
}

func TestValidateLicense_Missing(t *testing.T) {
	tempDir := t.TempDir()

	err := ValidateLicense(tempDir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin must contain LICENSE.txt file")
}

func TestValidatePluginScript_MissingLicense(t *testing.T) {
	tempDir := t.TempDir()

	manifest := PluginManifest{
		BasicPluginManifest: BasicPluginManifest{
			Name:           "test",
			Version:        "1.0.0",
			Description:    "Test plugin",
			Authentication: true,
		},
	}

	data, err := json.Marshal(manifest)
	require.NoError(t, err)

	writePluginManifestScript(t, tempDir, "dr-test", string(data))

	err = ValidatePluginScript(tempDir, manifest)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin must contain LICENSE.txt file")
}

func TestValidatePluginScript_WithLicense(t *testing.T) {
	tempDir := t.TempDir()

	manifest := PluginManifest{
		BasicPluginManifest: BasicPluginManifest{
			Name:           "test",
			Version:        "1.0.0",
			Description:    "Test plugin",
			Authentication: true,
		},
	}

	data, err := json.Marshal(manifest)
	require.NoError(t, err)

	writePluginManifestScript(t, tempDir, "dr-test", string(data))

	licensePath := filepath.Join(tempDir, "LICENSE.txt")
	err = os.WriteFile(licensePath, []byte("Apache License 2.0"), 0o644)
	require.NoError(t, err)

	err = ValidatePluginScript(tempDir, manifest)

	assert.NoError(t, err)
}
