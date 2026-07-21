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
	"slices"
	"syscall"
	"time"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/spf13/pflag"
)

// checkAndInstallPluginDeps checks and installs the plugin's declared dependencies
// before execution. Returns false if the check fails or the user declines installation.
//
// Confirmation modes (in order of precedence):
//  1. -y / --yes present in args
//  2. DATAROBOT_CLI_NON_INTERACTIVE env var set
//  3. Interactive [Y/n] prompt
func checkAndInstallPluginDeps(manifest PluginManifest, args []string) bool {
	confirm := func() bool { return confirmPluginDepsInstall(args) }

	if err := CheckAndInstallDeps(manifest.Name, confirm, os.Stderr); err != nil {
		if !errors.Is(err, ErrDepsDeclined) {
			fmt.Fprintf(os.Stderr, "plugin dependency check failed: %v\n", err)
		}

		return false
	}

	return true
}

// confirmPluginDepsInstall returns true when the user consents to installing
// missing plugin dependencies. Consent is granted automatically when -y/--yes
// is present in args or DATAROBOT_CLI_NON_INTERACTIVE is set; otherwise the
// user is prompted interactively.
func confirmPluginDepsInstall(args []string) bool {
	if slices.Contains(args, "-y") || slices.Contains(args, "--yes") || reader.IsNonInteractive() {
		return true
	}

	fmt.Fprint(os.Stdout, "Install missing dependencies? [Y/n]: ")

	return reader.AskYesNo()
}

// ExecutePlugin runs a plugin and returns its exit code.
// If the plugin manifest requires authentication, it will check/prompt for auth first.
// ctx is used to cancel the auth flow and subprocess if the user presses Ctrl-C.
// rootFlags is the root command's persistent flagset, used to forward annotated
// universal flags (e.g. --debug) to the plugin subprocess as DATAROBOT_CLI_* env vars.
func ExecutePlugin(ctx context.Context, manifest PluginManifest, executable string, args []string, rootFlags *pflag.FlagSet) int {
	if !checkAndInstallPluginDeps(manifest, args) {
		return 1
	}

	skipAuth := viperx.GetBool("skip-auth")

	if manifest.Authentication && !skipAuth {
		userAgent := fmt.Sprintf("DataRobot CLI plugin: %s (version %s)", manifest.Name, manifest.Version)
		authCtx := config.WithUserAgent(ctx, userAgent)

		if !auth.EnsureAuthenticated(authCtx) {
			return 1
		}
	}

	return executePluginCommand(ctx, executable, args, manifest.Authentication, rootFlags)
}

// executePluginCommand runs the actual plugin command.
func executePluginCommand(ctx context.Context, executable string, args []string, requireAuth bool, rootFlags *pflag.FlagSet) int {
	cmd := buildPluginCommand(ctx, executable, args, requireAuth, rootFlags)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			return exitErr.ExitCode()
		}

		return 1
	}

	return 0
}

// pluginCommandArgs returns the executable and argument list to invoke a plugin
// script. On Windows, .ps1 files are run through PowerShell, since Windows
// cannot execute a .ps1 text file directly.
func pluginCommandArgs(executable string, args ...string) (string, []string) {
	return pluginCommandArgsFor(runtime.GOOS, executable, args...)
}

// pluginCommandArgsFor is the goos-parameterized core of pluginCommandArgs,
// exposed as a test seam so the Windows-wrapping branch can be exercised on
// any platform.
func pluginCommandArgsFor(goos, executable string, args ...string) (string, []string) {
	if goos == "windows" && filepath.Ext(executable) == ".ps1" {
		psArgs := append([]string{"-ExecutionPolicy", "Bypass", "-File", executable}, args...)

		return "powershell.exe", psArgs
	}

	return executable, args
}

// buildPluginCommand creates the appropriate exec.Cmd for the given executable.
// On Windows, .ps1 files are executed via PowerShell.
// ctx cancellation sends SIGTERM to the process, with a 5-second grace
// period before SIGKILL.
func buildPluginCommand(ctx context.Context, executable string, args []string, requireAuth bool, rootFlags *pflag.FlagSet) *exec.Cmd {
	name, cmdArgs := pluginCommandArgs(executable, args...)

	cmd := exec.CommandContext(ctx, name, cmdArgs...)
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 5 * time.Second
	cmd.Env = buildPluginEnv(executable, requireAuth, rootFlags)

	return cmd
}

// universalFlagEnv returns "KEY=VALUE" strings for every persistent root flag
// carrying a config.UniversalAnnotationKey annotation. Values are read from
// viper (the authoritative source after flag parsing).
func universalFlagEnv(fs *pflag.FlagSet) []string {
	if fs == nil {
		return nil
	}

	var env []string

	fs.VisitAll(func(flag *pflag.Flag) {
		suffixes, ok := flag.Annotations[config.UniversalAnnotationKey]
		if !ok || len(suffixes) == 0 {
			return
		}

		envKey := config.EnvPrefix + suffixes[0]

		if flag.Value.Type() == "bool" {
			if viperx.GetBool(flag.Name) {
				env = append(env, envKey+"=1")
			}

			return
		}

		if val := viperx.GetString(flag.Name); val != "" {
			env = append(env, envKey+"="+val)
		}
	})

	return env
}

func buildPluginEnv(pluginPath string, requireAuth bool, rootFlags *pflag.FlagSet) []string {
	env := os.Environ()

	// Forward universal root flags (e.g. --debug, --disable-telemetry) as
	// DATAROBOT_CLI_* env vars so plugins can optionally honour them.
	// These override any inherited env vars of the same name.
	env = append(env, universalFlagEnv(rootFlags)...)

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
