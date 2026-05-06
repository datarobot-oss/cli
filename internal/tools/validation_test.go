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
	"bytes"
	"os"
	"runtime"
	"testing"

	"github.com/datarobot/cli/internal/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureLog redirects os.Stderr to a pipe, reinitializes the logger, runs fn,
// then returns everything written to the logger during fn's execution.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	origStderr := os.Stderr
	os.Stderr = w

	log.StartStderr()

	fn()

	w.Close()

	os.Stderr = origStderr

	t.Cleanup(log.StopStderr)

	var buf bytes.Buffer

	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	r.Close()

	return buf.String()
}

// TestVersionsYamlSchemaDefinition verifies the authoritative schema has the expected field rules.
func TestVersionsYamlSchemaDefinition(t *testing.T) {
	assert.True(t, versionsYamlSchema.Name.Required, "name must be required")
	assert.True(t, versionsYamlSchema.MinimumVersion.Required, "minimum-version must be required")
	assert.Equal(t, formatSemver, versionsYamlSchema.MinimumVersion.Format, "minimum-version must use semver format")
	assert.True(t, versionsYamlSchema.Command.Required, "command must be required")
	assert.True(t, versionsYamlSchema.URL.Required, "url must be required")
}

// TestInstallCommandsSchemaDefinition verifies platform rules: macOS and Linux required, Windows optional.
func TestInstallCommandsSchemaDefinition(t *testing.T) {
	assert.True(t, installCommandsSchema.MacOS.Required, "macOS install command must be required")
	assert.True(t, installCommandsSchema.Linux.Required, "Linux install command must be required")
	assert.False(t, installCommandsSchema.Windows.Required, "Windows install command must be optional")
}

func TestFieldRuleValidate(t *testing.T) {
	t.Run("required field missing — WARN logged", func(t *testing.T) {
		rule := FieldRule{Required: true}

		output := captureLog(t, func() {
			rule.validate("tool", "name", "")
		})

		assert.Contains(t, output, "WARN")
		assert.Contains(t, output, "[tool]")
		assert.Contains(t, output, "'name' is required")
	})

	t.Run("required field present — nothing logged", func(t *testing.T) {
		rule := FieldRule{Required: true}

		output := captureLog(t, func() {
			rule.validate("tool", "name", "Python")
		})

		assert.Empty(t, output)
	})

	t.Run("semver field with invalid version — WARN logged", func(t *testing.T) {
		rule := FieldRule{Format: formatSemver}

		output := captureLog(t, func() {
			rule.validate("tool", "minimum-version", "not-a-version")
		})

		assert.Contains(t, output, "WARN")
		assert.Contains(t, output, "[tool]")
		assert.Contains(t, output, "'minimum-version'")
		assert.Contains(t, output, "not a valid semantic version")
	})

	t.Run("semver field with valid version — nothing logged", func(t *testing.T) {
		rule := FieldRule{Format: formatSemver}

		output := captureLog(t, func() {
			rule.validate("tool", "minimum-version", "3.9.0")
		})

		assert.Empty(t, output)
	})

	t.Run("semver field empty and not required — nothing logged", func(t *testing.T) {
		rule := FieldRule{Format: formatSemver}

		output := captureLog(t, func() {
			rule.validate("tool", "minimum-version", "")
		})

		assert.Empty(t, output)
	})

	t.Run("optional field empty — nothing logged", func(t *testing.T) {
		rule := FieldRule{}

		output := captureLog(t, func() {
			rule.validate("tool", "command", "")
		})

		assert.Empty(t, output)
	})
}

func TestInstallCommandsSchemaValidate(t *testing.T) {
	schema := InstallCommandsSchema{
		MacOS:   FieldRule{Required: true},
		Linux:   FieldRule{Required: true},
		Windows: FieldRule{Required: false},
	}

	t.Run("all platforms absent — WARN 'install is not defined'", func(t *testing.T) {
		output := captureLog(t, func() {
			schema.validate("tool", InstallCommands{})
		})

		assert.Contains(t, output, "WARN")
		assert.Contains(t, output, "[tool]")
		assert.Contains(t, output, "'install' is not defined")
	})

	t.Run("all required platforms present — nothing logged", func(t *testing.T) {
		ic := InstallCommands{MacOS: "brew install x", Linux: "apt install x"}

		output := captureLog(t, func() {
			schema.validate("tool", ic)
		})

		assert.Empty(t, output)
	})

	t.Run("Windows optional and absent — nothing logged for Windows", func(t *testing.T) {
		ic := InstallCommands{MacOS: "brew install x", Linux: "apt install x"}

		output := captureLog(t, func() {
			schema.validate("tool", ic)
		})

		assert.NotContains(t, output, "install.windows")
	})

	t.Run("current platform missing — ERROR logged", func(t *testing.T) {
		var ic InstallCommands

		switch runtime.GOOS {
		case "darwin":
			ic = InstallCommands{Linux: "apt install x"}
		case "linux":
			ic = InstallCommands{MacOS: "brew install x"}
		default:
			t.Skip("current platform not covered by required fields in this schema")
		}

		output := captureLog(t, func() {
			schema.validate("tool", ic)
		})

		assert.Contains(t, output, "ERRO")
		assert.Contains(t, output, "[tool]")
		assert.Contains(t, output, "required for the current platform")
	})

	t.Run("non-current platform missing — WARN logged", func(t *testing.T) {
		var ic InstallCommands

		switch runtime.GOOS {
		case "darwin":
			ic = InstallCommands{MacOS: "brew install x"}
		case "linux":
			ic = InstallCommands{Linux: "apt install x"}
		default:
			t.Skip("current platform not covered by required fields in this schema")
		}

		output := captureLog(t, func() {
			schema.validate("tool", ic)
		})

		assert.Contains(t, output, "WARN")
		assert.Contains(t, output, "[tool]")
		assert.Contains(t, output, "is required")
	})
}

func TestYAMLSchemaValidate(t *testing.T) {
	t.Run("fully valid entry — nothing logged, nil returned", func(t *testing.T) {
		input := versionsYaml{
			"python": {
				Name: "Python", MinimumVersion: "3.9.0", Command: "python3", URL: "https://python.org",
				Install: InstallCommands{MacOS: "brew install python", Linux: "apt install python3"},
			},
		}

		output := captureLog(t, func() {
			versionsYamlSchema.Validate(input)
		})

		assert.Empty(t, output)
	})

	t.Run("missing required fields — WARN logged, nil returned", func(t *testing.T) {
		input := versionsYaml{
			"python": {},
		}

		output := captureLog(t, func() {
			versionsYamlSchema.Validate(input)
		})

		assert.Contains(t, output, "WARN")
		assert.Contains(t, output, "[python]")
	})

	t.Run("invalid semver — WARN logged, nil returned", func(t *testing.T) {
		input := versionsYaml{
			"python": {Name: "Python", Command: "python3", URL: "https://python.org", MinimumVersion: "bad"},
		}

		output := captureLog(t, func() {
			versionsYamlSchema.Validate(input)
		})

		assert.Contains(t, output, "WARN")
		assert.Contains(t, output, "not a valid semantic version")
	})

	t.Run("no install commands — WARN 'install is not defined', nil returned", func(t *testing.T) {
		input := versionsYaml{
			"python": {Name: "Python", MinimumVersion: "3.9.0", Command: "python3", URL: "https://python.org"},
		}

		output := captureLog(t, func() {
			versionsYamlSchema.Validate(input)
		})

		assert.Contains(t, output, "WARN")
		assert.Contains(t, output, "'install' is not defined")
	})

	t.Run("empty map — nothing logged, nil returned", func(t *testing.T) {
		output := captureLog(t, func() {
			versionsYamlSchema.Validate(versionsYaml{})
		})

		assert.Empty(t, output)
	})
}
