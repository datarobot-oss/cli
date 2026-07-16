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
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

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

// createScript writes content to path with executable permissions.
func createScript(t *testing.T, path, content string) {
	t.Helper()

	err := os.WriteFile(path, []byte(content), 0o755)
	require.NoError(t, err)
}
