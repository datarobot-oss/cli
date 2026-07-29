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

package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// versionsYAMLSatisfied has a dep that is always present and its version requirement is met.
const versionsYAMLSatisfied = `echo-tool:
  name: Echo tool
  minimum-version: "1.0.0"
  command: "echo 1.0.0"
  url: https://example.com
  install:
    macos: "echo install"
    linux: "echo install"
`

// versionsYAMLWrongVersion has a dep whose version requirement is never met,
// but the install command always exits 0 ("echo install").
const versionsYAMLWrongVersion = `echo-tool:
  name: Echo tool
  minimum-version: "99.0.0"
  command: "echo 1.0.0"
  url: https://example.com
  install:
    macos: "echo install"
    linux: "echo install"
    windows: "echo install"
`

// writePluginVersionsYAML creates a plugin dir under the managed plugins directory
// and writes the given YAML. Returns the plugin name and a cleanup function.
func writePluginVersionsYAML(t *testing.T, pluginName, yamlContent string) {
	t.Helper()

	managedDir, err := plugin.ManagedPluginsDir()
	require.NoError(t, err)

	pluginDir := filepath.Join(managedDir, pluginName)

	require.NoError(t, os.MkdirAll(pluginDir, 0o755))

	t.Cleanup(func() { _ = os.RemoveAll(pluginDir) })

	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "versions.yaml"), []byte(yamlContent), 0o644))
}

// --- confirmPluginDepsInstall ---

func TestConfirmPluginDepsInstall_YesFlag(t *testing.T) {
	origYesFlag := yesFlag

	defer func() { yesFlag = origYesFlag }()

	yesFlag = true

	assert.True(t, confirmPluginDepsInstall())
}

func TestConfirmPluginDepsInstall_ViperYes(t *testing.T) {
	defer viperx.Reset()

	viperx.Set("yes", true)

	assert.True(t, confirmPluginDepsInstall())
}

func TestConfirmPluginDepsInstall_UserAnswersY(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)

	_, _ = w.WriteString("y\n")
	w.Close()

	origStdin := os.Stdin
	os.Stdin = r

	defer func() {
		os.Stdin = origStdin

		r.Close()
	}()

	assert.True(t, confirmPluginDepsInstall())
}

func TestConfirmPluginDepsInstall_UserAnswersN(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)

	_, _ = w.WriteString("n\n")
	w.Close()

	origStdin := os.Stdin
	os.Stdin = r

	defer func() {
		os.Stdin = origStdin

		r.Close()
	}()

	assert.False(t, confirmPluginDepsInstall())
}

// --- checkAndInstallPluginDeps ---

func TestCheckAndInstallPluginDeps_SkipsWhenNoVersionsYaml(t *testing.T) {
	err := checkAndInstallPluginDeps("nonexistent-test-dr-cli-install-plugin-xyz")

	assert.NoError(t, err)
}

func TestCheckAndInstallPluginDeps_NilWhenAllDepsSatisfied(t *testing.T) {
	const pluginName = "test-dr-cli-install-satisfied"

	writePluginVersionsYAML(t, pluginName, versionsYAMLSatisfied)

	err := checkAndInstallPluginDeps(pluginName)

	assert.NoError(t, err)
}

func TestCheckAndInstallPluginDeps_SkipsInstallWhenUserDeclines(t *testing.T) {
	const pluginName = "test-dr-cli-install-decline"

	writePluginVersionsYAML(t, pluginName, versionsYAMLWrongVersion)

	origYesFlag := yesFlag

	defer func() { yesFlag = origYesFlag }()

	yesFlag = false

	r, w, err := os.Pipe()
	require.NoError(t, err)

	_, _ = w.WriteString("n\n")
	w.Close()

	origStdin := os.Stdin
	os.Stdin = r

	defer func() {
		os.Stdin = origStdin

		r.Close()
	}()

	installErr := checkAndInstallPluginDeps(pluginName)

	assert.NoError(t, installErr)
}

func TestCheckAndInstallPluginDeps_AutoConfirmsWithYesFlag(t *testing.T) {
	const pluginName = "test-dr-cli-install-yesflag"

	writePluginVersionsYAML(t, pluginName, versionsYAMLWrongVersion)

	origYesFlag := yesFlag

	defer func() { yesFlag = origYesFlag }()

	yesFlag = true

	err := checkAndInstallPluginDeps(pluginName)

	assert.NoError(t, err)
}

func TestCheckAndInstallPluginDeps_AutoConfirmsWithViperYes(t *testing.T) {
	const pluginName = "test-dr-cli-install-viper-yes"

	writePluginVersionsYAML(t, pluginName, versionsYAMLWrongVersion)

	origYesFlag := yesFlag

	defer func() {
		yesFlag = origYesFlag

		viperx.Reset()
	}()

	yesFlag = false

	viperx.Set("yes", true)

	err := checkAndInstallPluginDeps(pluginName)

	assert.NoError(t, err)
}
