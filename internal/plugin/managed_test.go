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
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// installFakeManagedPlugin writes a minimal managed plugin (manifest.json plus
// both platform scripts) under dir and returns the plugin directory.
func installFakeManagedPlugin(t *testing.T, dir, name string) string {
	t.Helper()

	pluginDir := filepath.Join(dir, "datarobot", "plugins", name)

	require.NoError(t, os.MkdirAll(pluginDir, 0o755))

	manifest := `{"name":"` + name + `","version":"1.0.0","scripts":{"posix":"run.sh","windows":"run.ps1"}}`

	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "manifest.json"), []byte(manifest), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "run.sh"), []byte("#!/bin/sh\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "run.ps1"), []byte(""), 0o644))

	return pluginDir
}

func TestLoadManagedPlugin_NotInstalled(t *testing.T) {
	testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", t.TempDir())

	p, err := LoadManagedPlugin("no-such-plugin")

	require.NoError(t, err)
	assert.Nil(t, p, "a missing plugin should resolve to nil without an error")
}

func TestLoadManagedPlugin_ResolvesManifestAndExecutable(t *testing.T) {
	tmpXDG := t.TempDir()

	testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", tmpXDG)

	pluginDir := installFakeManagedPlugin(t, tmpXDG, "studio-test")

	p, err := LoadManagedPlugin("studio-test")

	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "studio-test", p.Manifest.Name)
	assert.Equal(t, "1.0.0", p.Manifest.Version)
	assert.Equal(t, pluginDir, filepath.Dir(p.Executable), "executable should live in the plugin directory")
}

func TestLoadManagedPlugin_MissingScriptIsAnError(t *testing.T) {
	tmpXDG := t.TempDir()

	testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", tmpXDG)

	pluginDir := installFakeManagedPlugin(t, tmpXDG, "broken-test")

	require.NoError(t, os.Remove(filepath.Join(pluginDir, "run.sh")))
	require.NoError(t, os.Remove(filepath.Join(pluginDir, "run.ps1")))

	_, err := LoadManagedPlugin("broken-test")

	assert.Error(t, err, "an installed plugin whose script is missing should surface an error")
}
