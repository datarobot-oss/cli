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

package pluginpackage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writePackagableFixture builds a plugin directory in the shape that exposed
// ALIFE-994: extensionless dr-* platform binaries that sort alphabetically
// ahead of the real entry script, alongside a manifest.json whose scripts
// block declares the entry scripts. When declaredVersion is empty the entry
// script echoes the manifest's own values (consistent package); otherwise it
// echoes the given version (stale embedded manifest, as shipped in codespace
// 0.6.0). Returns the plugin dir and output dir paths.
func writePackagableFixture(t *testing.T, declaredVersion string) (string, string) {
	t.Helper()

	pluginDir := t.TempDir()

	outDir := t.TempDir()

	const (
		name        = "pkgtest"
		version     = "1.0.0"
		description = "Packaging fixture plugin"
	)

	manifestJSON, err := json.Marshal(map[string]any{
		"name":           name,
		"version":        version,
		"description":    description,
		"authentication": true,
		"scripts": map[string]string{
			"posix":   "dr-pkgtest.sh",
			"windows": "dr-pkgtest.ps1",
		},
	})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "manifest.json"), manifestJSON, 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "LICENSE.txt"), []byte("Apache License 2.0"), 0o644))

	// Extensionless dr-* binaries alphabetically ahead of the entry script.
	// If validation regresses to the directory scan, packaging executes one of
	// these and fails instead of reaching the archive step.
	for _, bin := range []string{"dr-pkgtest-linux-x64", "dr-pkgtest-macos-arm64"} {
		require.NoError(t, os.WriteFile(filepath.Join(pluginDir, bin), []byte("not a script"), 0o755))
	}

	// The entry scripts echo the manifest the package claims to ship. Both
	// platforms are written: packages are platform-agnostic, and validation
	// requires both declared scripts on disk.
	echoVersion := version

	if declaredVersion != "" {
		echoVersion = declaredVersion
	}

	shScript := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--dr-plugin-manifest\" ]; then\n" +
		"  echo '{\"name\":\"" + name + "\",\"version\":\"" + echoVersion + "\",\"description\":\"" + description + "\",\"authentication\":true}'\n" +
		"  exit 0\n" +
		"fi\n"

	ps1Script := "if ($args[0] -eq '--dr-plugin-manifest') {\n" +
		"  Write-Output '{\"name\":\"" + name + "\",\"version\":\"" + echoVersion + "\",\"description\":\"" + description + "\",\"authentication\":true}'\n" +
		"}\n"

	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "dr-pkgtest.sh"), []byte(shScript), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "dr-pkgtest.ps1"), []byte(ps1Script), 0o755))

	return pluginDir, outDir
}

// TestPackagePlugin_UsesDeclaredScript is the command-level regression net for
// ALIFE-994: a codespace-style archive (extensionless dr-* binaries sorting
// ahead of the real entry script) must package via the manifest-declared
// entry script, not the directory scan.
func TestPackagePlugin_UsesDeclaredScript(t *testing.T) {
	pluginDir, outDir := writePackagableFixture(t, "")

	err := packagePlugin(pluginDir, outDir, "")

	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(outDir, "pkgtest-1.0.0.tar.xz"))

	require.NoError(t, err)
	assert.Positive(t, info.Size(), "archive should not be empty")
}

// TestPackagePlugin_MismatchedScriptManifest covers the round-trip
// consistency rule the codespace 0.6.0 release violated: the entry script's
// --dr-plugin-manifest output must agree with manifest.json.
func TestPackagePlugin_MismatchedScriptManifest(t *testing.T) {
	pluginDir, outDir := writePackagableFixture(t, "9.9.9")

	err := packagePlugin(pluginDir, outDir, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match")
}

// TestPackagePlugin_MissingDeclaredScript verifies a scripts block pointing at
// a file that is not in the package is a hard error, not a silent fallback to
// the dr-* directory scan.
func TestPackagePlugin_MissingDeclaredScript(t *testing.T) {
	pluginDir, outDir := writePackagableFixture(t, "")

	// Remove the current platform's declared entry script after fixture setup.
	entry := filepath.Join(pluginDir, "dr-pkgtest.sh")

	if runtime.GOOS == "windows" {
		entry = filepath.Join(pluginDir, "dr-pkgtest.ps1")
	}

	require.NoError(t, os.Remove(entry))

	err := packagePlugin(pluginDir, outDir, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "manifest.json declares script")
}
