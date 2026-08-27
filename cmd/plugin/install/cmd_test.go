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

	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/plugin"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeCmd builds a minimal *cobra.Command with the --yes/-y flag registered,
// parsing --yes onto it when yes is true. It is used to exercise
// confirmPluginDepsInstall and checkAndInstallPluginDeps without relying on
// the package-level flag state that was removed.
func makeCmd(t *testing.T, yes bool) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{Use: "install"}

	cmd.Flags().BoolP(cli.YesFlagName, "y", false, "")

	if yes {
		require.NoError(t, cmd.ParseFlags([]string{"--yes"}))
	}

	return cmd
}

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
	assert.True(t, confirmPluginDepsInstall(makeCmd(t, true)))
}

func TestConfirmPluginDepsInstall_ViperYes(t *testing.T) {
	defer viperx.Reset()

	viperx.Set("yes", true)

	// IsNonInteractive falls back to viperx.GetBool(YesFlagName) when the
	// --yes flag is not set on the command, so a cmd without --yes still
	// resolves to non-interactive here.
	assert.True(t, confirmPluginDepsInstall(makeCmd(t, false)))
}

func TestConfirmPluginDepsInstall_UserAnswersY(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)

	_, _ = w.WriteString("y\n")
	w.Close()

	origStdin := reader.Stdin
	reader.Stdin = r

	defer func() {
		reader.Stdin = origStdin

		r.Close()
	}()

	assert.True(t, confirmPluginDepsInstall(makeCmd(t, false)))
}

func TestConfirmPluginDepsInstall_UserAnswersN(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)

	_, _ = w.WriteString("n\n")
	w.Close()

	origStdin := reader.Stdin
	reader.Stdin = r

	defer func() {
		reader.Stdin = origStdin

		r.Close()
	}()

	assert.False(t, confirmPluginDepsInstall(makeCmd(t, false)))
}

// --- checkAndInstallPluginDeps ---

func TestCheckAndInstallPluginDeps_SkipsWhenNoVersionsYaml(t *testing.T) {
	err := checkAndInstallPluginDeps(makeCmd(t, false), "nonexistent-test-dr-cli-install-plugin-xyz")

	assert.NoError(t, err)
}

func TestCheckAndInstallPluginDeps_NilWhenAllDepsSatisfied(t *testing.T) {
	const pluginName = "test-dr-cli-install-satisfied"

	writePluginVersionsYAML(t, pluginName, versionsYAMLSatisfied)

	err := checkAndInstallPluginDeps(makeCmd(t, false), pluginName)

	assert.NoError(t, err)
}

func TestCheckAndInstallPluginDeps_SkipsInstallWhenUserDeclines(t *testing.T) {
	const pluginName = "test-dr-cli-install-decline"

	writePluginVersionsYAML(t, pluginName, versionsYAMLWrongVersion)

	r, w, err := os.Pipe()
	require.NoError(t, err)

	_, _ = w.WriteString("n\n")
	w.Close()

	origStdin := reader.Stdin
	reader.Stdin = r

	defer func() {
		reader.Stdin = origStdin

		r.Close()
	}()

	installErr := checkAndInstallPluginDeps(makeCmd(t, false), pluginName)

	assert.NoError(t, installErr)
}

func TestCheckAndInstallPluginDeps_AutoConfirmsWithYesFlag(t *testing.T) {
	const pluginName = "test-dr-cli-install-yesflag"

	writePluginVersionsYAML(t, pluginName, versionsYAMLWrongVersion)

	err := checkAndInstallPluginDeps(makeCmd(t, true), pluginName)

	assert.NoError(t, err)
}

func TestCheckAndInstallPluginDeps_AutoConfirmsWithViperYes(t *testing.T) {
	const pluginName = "test-dr-cli-install-viper-yes"

	writePluginVersionsYAML(t, pluginName, versionsYAMLWrongVersion)

	defer viperx.Reset()

	viperx.Set("yes", true)

	err := checkAndInstallPluginDeps(makeCmd(t, false), pluginName)

	assert.NoError(t, err)
}
