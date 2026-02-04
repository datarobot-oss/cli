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
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config"
	"github.com/spf13/viper"
)

// ExecutePlugin runs a plugin and returns its exit code
// If the plugin manifest requires authentication, it will check/prompt for auth first
func ExecutePlugin(manifest PluginManifest, executable string, args []string) int {
	// Check authentication if required by the plugin
	if manifest.Authentication && !auth.EnsureAuthenticated(context.Background()) {
		return 1
	}

	return executePluginCommand(executable, args, manifest.Authentication)
}

// executePluginCommand runs the actual plugin command
func executePluginCommand(executable string, args []string, requireAuth bool) int {
	cmd := buildPluginCommand(executable, args, requireAuth)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Forward signals to child process with cleanup
	sigChan := make(chan os.Signal, 1)
	done := make(chan struct{})

	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case sig := <-sigChan:
			if cmd.Process != nil {
				_ = cmd.Process.Signal(sig)
			}
		case <-done:
			// Command completed, exit goroutine cleanly
			return
		}
	}()

	err := cmd.Run()

	// Signal goroutine to exit and cleanup
	close(done)
	signal.Stop(sigChan)

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}

		return 1
	}

	return 0
}

// buildPluginCommand creates the appropriate exec.Cmd for the given executable
// On Windows, .ps1 files are executed via PowerShell
func buildPluginCommand(executable string, args []string, requireAuth bool) *exec.Cmd {
	ext := filepath.Ext(executable)

	// On Windows, execute .ps1 files through PowerShell
	if runtime.GOOS == "windows" && ext == ".ps1" {
		psArgs := append([]string{"-ExecutionPolicy", "Bypass", "-File", executable}, args...)

		cmd := exec.Command("powershell.exe", psArgs...)
		cmd.Env = buildPluginEnv(requireAuth)
		return cmd
	}

	cmd := exec.Command(executable, args...)
	cmd.Env = buildPluginEnv(requireAuth)

	return cmd
}

func buildPluginEnv(requireAuth bool) []string {
	env := os.Environ()

	// Always set plugin mode flag so plugins can detect they were invoked by dr CLI
	env = append(env, "DR_PLUGIN_MODE=1")

	if !requireAuth {
		return env
	}

	if configPath := viper.ConfigFileUsed(); configPath != "" {
		env = append(env, "DATAROBOT_CONFIG="+configPath)
	}

	if endpoint := viper.GetString(config.DataRobotURL); endpoint != "" {
		env = append(env, "DATAROBOT_ENDPOINT="+endpoint)
	}

	if token := viper.GetString(config.DataRobotAPIKey); token != "" {
		env = append(env, "DATAROBOT_API_TOKEN="+token)
	}

	return env
}
