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
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
)

// ExecutePlugin runs a plugin and returns its exit code.
// If the plugin manifest requires authentication, it will check/prompt for auth first.
// ctx is used to cancel the auth flow and subprocess if the user presses Ctrl-C.
func ExecutePlugin(ctx context.Context, manifest PluginManifest, executable string, args []string) int {
	if manifest.Authentication {
		userAgent := fmt.Sprintf("DataRobot CLI plugin: %s (version %s)", manifest.Name, manifest.Version)
		authCtx := config.WithUserAgent(ctx, userAgent)

		if !auth.EnsureAuthenticated(authCtx) {
			return 1
		}
	}

	return executePluginCommand(ctx, executable, args, manifest.Authentication)
}

// executePluginCommand runs the actual plugin command.
// ctx cancellation kills the subprocess (e.g. on Ctrl-C).
func executePluginCommand(ctx context.Context, executable string, args []string, requireAuth bool) int {
	cmd := buildPluginCommand(ctx, executable, args, requireAuth)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}

		return 1
	}

	return 0
}

// buildPluginCommand creates the appropriate exec.Cmd for the given executable.
// On Windows, .ps1 files are executed via PowerShell.
// ctx cancellation sends SIGKILL to the process.
func buildPluginCommand(ctx context.Context, executable string, args []string, requireAuth bool) *exec.Cmd {
	ext := filepath.Ext(executable)

	// On Windows, execute .ps1 files through PowerShell
	if runtime.GOOS == "windows" && ext == ".ps1" {
		psArgs := append([]string{"-ExecutionPolicy", "Bypass", "-File", executable}, args...)

		cmd := exec.CommandContext(ctx, "powershell.exe", psArgs...)
		cmd.Env = buildPluginEnv(executable, requireAuth)

		return cmd
	}

	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Env = buildPluginEnv(executable, requireAuth)

	return cmd
}

func buildPluginEnv(pluginPath string, requireAuth bool) []string {
	env := os.Environ()

	// Always set plugin mode flag so plugins can detect they were invoked by dr CLI
	env = append(env, "DR_PLUGIN_MODE=1")

	// Set the path to the plugin executable
	if pluginPath != "" {
		env = append(env, "DR_PLUGIN_PATH="+pluginPath)
	}

	// Set config path for all plugins
	if configPath := viperx.ConfigFileUsed(); configPath != "" {
		env = append(env, "DATAROBOT_CONFIG="+configPath)
	}

	if !requireAuth {
		return env
	}

	if endpoint := viperx.GetString(config.DataRobotURL); endpoint != "" {
		env = append(env, "DATAROBOT_ENDPOINT="+endpoint)
	}

	if token := viperx.GetString(config.DataRobotAPIKey); token != "" {
		env = append(env, "DATAROBOT_API_TOKEN="+token)
	}

	return env
}
