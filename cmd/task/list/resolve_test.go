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

package list

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	rootTaskfileYAML     = "Taskfile.yaml"
	rootTaskfileYML      = "Taskfile.yml"
	generatedTaskfileYML = "Taskfile.gen.yaml"

	// projectTemplateMarker only ever reaches a generated Taskfile via the
	// project's own .Taskfile.template, so finding it proves the embedded
	// default was not used.
	projectTemplateMarker = "from-project-template"
)

// `dr task list` has to read the same Taskfile `dr run` executes. Resolving it
// separately made list regenerate Taskfile.gen.yaml over the file run had just
// chosen, and list a different set of tasks than the next run would execute.
func TestCmdListsRootTaskfileWithoutRegenerating(t *testing.T) {
	projectDir := t.TempDir()
	writeProjectFixture(t, projectDir, true)

	logFile := filepath.Join(t.TempDir(), "task.log")
	writeFakeTaskBinary(t, logFile)

	cmd := Cmd()
	cmd.SetArgs([]string{"--dir", projectDir})

	require.NoError(t, cmd.Execute())

	require.Contains(t, readTextFile(t, logFile), "-t "+filepath.Join(projectDir, rootTaskfileYML))

	_, err := os.Stat(filepath.Join(projectDir, generatedTaskfileYML))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCmdListsTaskfileGeneratedFromProjectTemplate(t *testing.T) {
	projectDir := t.TempDir()
	writeProjectFixture(t, projectDir, false)

	logFile := filepath.Join(t.TempDir(), "task.log")
	writeFakeTaskBinary(t, logFile)

	cmd := Cmd()
	cmd.SetArgs([]string{"--dir", projectDir})

	require.NoError(t, cmd.Execute())

	require.Contains(t, readTextFile(t, logFile), "-t "+filepath.Join(projectDir, generatedTaskfileYML))

	generated := readTextFile(t, filepath.Join(projectDir, generatedTaskfileYML))
	require.Contains(t, generated, projectTemplateMarker)
}

func writeProjectFixture(t *testing.T, projectDir string, includeRootTaskfile bool) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".datarobot", "answers"), 0o755))

	template := strings.Join([]string{
		"version: '3'",
		"includes:",
		"  {{- range .Includes }}",
		"  {{ .Name }}:",
		"    taskfile: {{ .Taskfile }}",
		"    dir: {{ .Dir }}",
		"  {{- end }}",
		"tasks:",
		"  " + projectTemplateMarker + ":",
		"    desc: Marker task",
		"    cmds:",
		"      - echo marker",
		"",
	}, "\n")

	require.NoError(t, os.WriteFile(
		filepath.Join(projectDir, ".Taskfile.template"), []byte(template), 0o644))

	if includeRootTaskfile {
		root := "version: '3'\ntasks:\n  committed:\n    desc: Committed task\n    cmds:\n      - echo committed\n"
		require.NoError(t, os.WriteFile(filepath.Join(projectDir, rootTaskfileYML), []byte(root), 0o644))
	}

	writeComponentTaskfile(t, filepath.Join(projectDir, "agent"), rootTaskfileYML)
	writeComponentTaskfile(t, filepath.Join(projectDir, "fastapi_server"), rootTaskfileYAML)
}

func writeComponentTaskfile(t *testing.T, componentDir string, filename string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	contents := strings.Join([]string{
		"version: '3'",
		"tasks:",
		"  dev:",
		"    desc: Dev component",
		"    cmds:",
		"      - echo dev",
		"",
	}, "\n")

	require.NoError(t, os.WriteFile(filepath.Join(componentDir, filename), []byte(contents), 0o644))
}

func writeFakeTaskBinary(t *testing.T, logFile string) {
	t.Helper()

	binDir := t.TempDir()
	t.Setenv("TASK_LOG", logFile)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if runtime.GOOS == "windows" {
		writeFakeWindowsTaskBinary(t, binDir)

		return
	}

	writeFakeUnixTaskBinary(t, binDir)
}

func writeFakeUnixTaskBinary(t *testing.T, binDir string) {
	t.Helper()

	script := `#!/bin/sh
printf '%s\n' "$*" >> "$TASK_LOG"
if [ "$1" = "--list" ]; then
  cat <<'JSON'
{"tasks":[{"name":"dev","desc":"Dev component"}]}
JSON
fi
exit 0
`

	require.NoError(t, os.WriteFile(filepath.Join(binDir, "task"), []byte(script), 0o755))
}

func writeFakeWindowsTaskBinary(t *testing.T, binDir string) {
	t.Helper()

	script := `@echo off
echo %*>>"%TASK_LOG%"
if "%1"=="--list" (
  echo {"tasks":[{"name":"dev","desc":"Dev component"}]}
)
exit /b 0
`

	require.NoError(t, os.WriteFile(filepath.Join(binDir, "task.bat"), []byte(script), 0o755))
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()

	contents, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(contents)
}
