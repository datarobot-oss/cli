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

package run

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
)

func TestCmdPrefersRecipeRootTaskfileWhenPresent(t *testing.T) {
	recipeDir := t.TempDir()
	writeRecipeFixture(t, recipeDir, true)

	logFile := filepath.Join(t.TempDir(), "task.log")
	writeFakeTaskBinary(t, logFile)

	cmd := Cmd()
	cmd.SetArgs([]string{"--dir", recipeDir, "start"})

	require.NoError(t, cmd.Execute())

	logs := readTaskLog(t, logFile)
	require.Contains(t, logs, "-t "+filepath.Join(recipeDir, rootTaskfileYML)+" -C 2 start")
	require.Contains(t, logs, "|1|")

	_, err := os.Stat(filepath.Join(recipeDir, generatedTaskfileYML))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCmdUsesEmbeddedGeneratedTaskfileWhenCalledFromRootTaskfile(t *testing.T) {
	recipeDir := t.TempDir()
	writeRecipeFixture(t, recipeDir, true)
	t.Setenv(taskRunFromRootEnv, "1")

	logFile := filepath.Join(t.TempDir(), "task.log")
	writeFakeTaskBinary(t, logFile)

	cmd := Cmd()
	cmd.SetArgs([]string{"--dir", recipeDir, "start"})

	require.NoError(t, cmd.Execute())

	logs := readTaskLog(t, logFile)
	require.Contains(t, logs, "-t "+filepath.Join(recipeDir, generatedTaskfileYML)+" -C 2 start")
	require.NotContains(t, logs, "-t "+filepath.Join(recipeDir, rootTaskfileYML)+" -C 2 start")

	generated := readTextFile(t, filepath.Join(recipeDir, generatedTaskfileYML))
	require.Contains(t, generated, "task agent:start")
	require.NotContains(t, generated, "dr task run start")
	require.NotContains(t, generated, "build-agents-md")
}

func TestCmdUsesRecipeTemplateWhenRootTaskfileIsMissing(t *testing.T) {
	recipeDir := t.TempDir()
	writeRecipeFixture(t, recipeDir, false)

	logFile := filepath.Join(t.TempDir(), "task.log")
	writeFakeTaskBinary(t, logFile)

	cmd := Cmd()
	cmd.SetArgs([]string{"--dir", recipeDir, "dev"})

	require.NoError(t, cmd.Execute())

	logs := readTaskLog(t, logFile)
	require.Contains(t, logs, "-t "+filepath.Join(recipeDir, generatedTaskfileYML)+" -C 2 dev")

	generated := readTextFile(t, filepath.Join(recipeDir, generatedTaskfileYML))
	require.Contains(t, generated, "build-agents-md")
	require.Contains(t, generated, "drdev")
}

func writeRecipeFixture(t *testing.T, recipeDir string, includeRootTaskfile bool) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Join(recipeDir, ".datarobot", "answers"), 0o755))
	copyTestFixture(t, filepath.Join("recipe", ".Taskfile.template"), filepath.Join(recipeDir, ".Taskfile.template"))

	if includeRootTaskfile {
		copyTestFixture(t, filepath.Join("recipe", rootTaskfileYML), filepath.Join(recipeDir, rootTaskfileYML))
	}

	writeComponentTaskfile(t, filepath.Join(recipeDir, "agent"), rootTaskfileYML)
	writeComponentTaskfile(t, filepath.Join(recipeDir, "fastapi_server"), rootTaskfileYAML)
	writeComponentTaskfile(t, filepath.Join(recipeDir, "infra"), rootTaskfileYAML)
}

func copyTestFixture(t *testing.T, fixturePath string, destination string) {
	t.Helper()

	contents := readTextFile(t, filepath.Join("testdata", fixturePath))
	require.NoError(t, os.WriteFile(destination, []byte(contents), 0o644))
}

func writeComponentTaskfile(t *testing.T, componentDir string, filename string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	contents := strings.Join([]string{
		"version: '3'",
		"tasks:",
		"  start:",
		"    desc: Start component",
		"    cmds:",
		"      - echo start",
		"  lint:",
		"    desc: Lint component",
		"    cmds:",
		"      - echo lint",
		"  install:",
		"    desc: Install component",
		"    cmds:",
		"      - echo install",
		"  test:",
		"    desc: Test component",
		"    cmds:",
		"      - echo test",
		"  dev:",
		"    desc: Dev component",
		"    cmds:",
		"      - echo dev",
		"  deploy-dev:",
		"    aliases: [up-dev]",
		"    desc: Deploy dev component",
		"    cmds:",
		"      - echo deploy-dev",
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
if [ "$1" = "--list" ]; then
  cat <<'JSON'
{"tasks":[{"name":"start","desc":"Start component"},{"name":"lint","desc":"Lint component"},{"name":"install","desc":"Install component"},{"name":"test","desc":"Test component"},{"name":"dev","desc":"Dev component"},{"name":"deploy-dev","desc":"Deploy dev component","aliases":["up-dev"]}]}
JSON
  exit 0
fi

printf '%s|%s|%s\n' "$PWD" "$DATAROBOT_CLI_TASK_RUN_FROM_ROOT" "$*" >> "$TASK_LOG"
`

	require.NoError(t, os.WriteFile(filepath.Join(binDir, "task"), []byte(script), 0o755))
}

func writeFakeWindowsTaskBinary(t *testing.T, binDir string) {
	t.Helper()

	script := `@echo off
if "%1"=="--list" (
  echo {"tasks":[{"name":"start","desc":"Start component"},{"name":"lint","desc":"Lint component"},{"name":"install","desc":"Install component"},{"name":"test","desc":"Test component"},{"name":"dev","desc":"Dev component"},{"name":"deploy-dev","desc":"Deploy dev component","aliases":["up-dev"]}]}
  exit /b 0
)
echo %CD%^|%DATAROBOT_CLI_TASK_RUN_FROM_ROOT%^|%*>>"%TASK_LOG%"
exit /b 0
`

	require.NoError(t, os.WriteFile(filepath.Join(binDir, "task.bat"), []byte(script), 0o755))
}

func readTaskLog(t *testing.T, logFile string) string {
	t.Helper()

	return readTextFile(t, logFile)
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()

	contents, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(contents)
}
