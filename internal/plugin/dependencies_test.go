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
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// depsSatisfied: echo outputs "1.0.0", minimum is "1.0.0" — always passes.
	depsSatisfied = `echo-tool:
  name: Echo tool
  minimum-version: "1.0.0"
  command: "echo 1.0.0"
  url: https://example.com
  install:
    macos: "echo install"
    linux: "echo install"
`

	// depsWrongVersion: echo outputs "1.0.0", minimum is "99.0.0" — always fails.
	// Install command exits 0 so tests that proceed past the prompt succeed.
	depsWrongVersion = `echo-tool:
  name: Echo tool
  minimum-version: "99.0.0"
  command: "echo 1.0.0"
  url: https://example.com
  install:
    macos: "echo install"
    linux: "echo install"
`
)

// writeDepVersionsYAML creates a managed plugin dir for pluginName and writes
// yamlContent as its versions.yaml. Cleanup removes the dir after the test.
func writeDepVersionsYAML(t *testing.T, pluginName, yamlContent string) {
	t.Helper()

	managedDir, err := ManagedPluginsDir()
	require.NoError(t, err)

	pluginDir := filepath.Join(managedDir, pluginName)

	require.NoError(t, os.MkdirAll(pluginDir, 0o755))

	t.Cleanup(func() { _ = os.RemoveAll(pluginDir) })

	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "versions.yaml"), []byte(yamlContent), 0o644))
}

// neverConfirm is a confirm func that must not be called (fails the test if it is).
func neverConfirm(t *testing.T) func() bool {
	t.Helper()

	return func() bool {
		t.Error("confirm should not have been called")

		return false
	}
}

// --- CheckAndInstallDeps ---

func TestCheckAndInstallDeps_NilWhenNoVersionsYaml(t *testing.T) {
	err := CheckAndInstallDeps("nonexistent-test-dr-cli-deps-plugin-xyz", neverConfirm(t), io.Discard)

	assert.NoError(t, err)
}

func TestCheckAndInstallDeps_NilWhenVersionsYamlMissing(t *testing.T) {
	// Plugin dir exists but contains no versions.yaml — GetRequirementsFromDir
	// returns os.ErrNotExist, which CheckAndInstallDeps must swallow silently.
	const pluginName = "test-dr-cli-deps-no-yaml"

	managedDir, err := ManagedPluginsDir()
	require.NoError(t, err)

	pluginDir := filepath.Join(managedDir, pluginName)

	require.NoError(t, os.MkdirAll(pluginDir, 0o755))

	t.Cleanup(func() { _ = os.RemoveAll(pluginDir) })

	err = CheckAndInstallDeps(pluginName, neverConfirm(t), io.Discard)

	assert.NoError(t, err)
}

func TestCheckAndInstallDeps_NilWhenAllSatisfied(t *testing.T) {
	const pluginName = "test-dr-cli-deps-satisfied"

	writeDepVersionsYAML(t, pluginName, depsSatisfied)

	err := CheckAndInstallDeps(pluginName, neverConfirm(t), io.Discard)

	assert.NoError(t, err)
}

func TestCheckAndInstallDeps_ErrDeclinedWhenConfirmFalse(t *testing.T) {
	const pluginName = "test-dr-cli-deps-declined"

	writeDepVersionsYAML(t, pluginName, depsWrongVersion)

	err := CheckAndInstallDeps(pluginName, func() bool { return false }, io.Discard)

	assert.ErrorIs(t, err, ErrDepsDeclined)
}

func TestCheckAndInstallDeps_NilAfterSuccessfulInstall(t *testing.T) {
	const pluginName = "test-dr-cli-deps-install-ok"

	writeDepVersionsYAML(t, pluginName, depsWrongVersion)

	err := CheckAndInstallDeps(pluginName, func() bool { return true }, io.Discard)

	assert.NoError(t, err)
}

func TestCheckAndInstallDeps_PropagatesInvalidYAML(t *testing.T) {
	const pluginName = "test-dr-cli-deps-bad-yaml"

	managedDir, err := ManagedPluginsDir()
	require.NoError(t, err)

	pluginDir := filepath.Join(managedDir, pluginName)

	require.NoError(t, os.MkdirAll(pluginDir, 0o755))

	t.Cleanup(func() { _ = os.RemoveAll(pluginDir) })

	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "versions.yaml"), []byte(":\tinvalid: yaml [[["), 0o644))

	err = CheckAndInstallDeps(pluginName, neverConfirm(t), io.Discard)

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrDepsDeclined)
}
