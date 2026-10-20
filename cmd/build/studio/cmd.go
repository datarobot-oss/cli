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

package studio

import (
	"errors"
	"fmt"
	"os"

	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/plugin"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

const (
	xpPluginName       = "xp"
	openCodePluginName = "opencode"

	// enableAssistFlag keeps the Assist tab available on xp versions that do
	// not enable it by default yet; newer versions treat it as a no-op.
	enableAssistFlag = "--enable-assist"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "studio",
		Short: "🎨 Open Build Studio (the dr xp Assist tab)",
		Long: `Open Build Studio — the Assist tab of the dr xp experimentation dashboard.

When the xp or opencode plugin is missing, it is installed from the plugin
registry first. Arguments after -- are passed through to dr xp.`,
		Example: `  dr build studio
  dr build studio -- --port 9000`,
		Args: cobra.ArbitraryArgs,
		RunE: run,
	}

	cmd.Flags().BoolP(cli.YesFlagName, "y", false, `Assume "yes" when prompted to install plugin dependencies.`)

	_ = viperx.BindEnv(cli.YesFlagName, "DATAROBOT_CLI_NON_INTERACTIVE")

	telemetry.Track(cmd)

	return cmd
}

func run(cmd *cobra.Command, args []string) error {
	xp, err := ensureStudioPlugins(cmd)
	if err != nil {
		return err
	}

	fmt.Println(tui.InfoStyle.Render("🔌 Running plugin: " + xpPluginName))

	pluginArgs := append([]string{enableAssistFlag}, args...)
	exitCode := plugin.ExecutePlugin(cmd.Context(), xp.Manifest, xp.Executable, pluginArgs, cmd.Root().PersistentFlags())

	telemetry.ExitWithContext(cmd.Context(), exitCode)

	return nil
}

// ensureStudioPlugins makes sure both plugins Build Studio needs are
// installed, fetching the plugin registry at most once, and returns the xp
// plugin to execute. The opencode plugin is only installed, not executed:
// xp drives it to power the Assist tab.
func ensureStudioPlugins(cmd *cobra.Command) (*plugin.DiscoveredPlugin, error) {
	xp := resolveInstalled(xpPluginName)

	var missing []string

	if xp == nil {
		missing = append(missing, xpPluginName)
	}

	if resolveInstalled(openCodePluginName) == nil {
		missing = append(missing, openCodePluginName)
	}

	if len(missing) == 0 {
		return xp, nil
	}

	if err := installFromRegistry(cmd, missing); err != nil {
		return nil, err
	}

	if xp != nil {
		return xp, nil
	}

	xp, err := plugin.LoadManagedPlugin(xpPluginName)
	if err != nil {
		return nil, fmt.Errorf("failed to load plugin %q after installation: %w", xpPluginName, err)
	}

	if xp == nil {
		return nil, fmt.Errorf("plugin %q is not available after installation", xpPluginName)
	}

	return xp, nil
}

// resolveInstalled returns the installed plugin with the given name, or nil
// when it is not installed. It consults cached discovery results first (which
// also cover PATH-based plugins) and falls back to reading the managed
// plugins directory directly, so an installed plugin is still found when
// discovery is disabled or timed out. A broken managed installation resolves
// to nil: reinstalling repairs it.
func resolveInstalled(name string) *plugin.DiscoveredPlugin {
	if p := findInstalled(name); p != nil {
		return p
	}

	p, err := plugin.LoadManagedPlugin(name)
	if err != nil {
		log.Debug("Failed to load managed plugin, will reinstall", "plugin", name, "error", err)

		return nil
	}

	return p
}

// findInstalled returns the discovered plugin with the given name, or nil
// when it is not installed. Discovery results are cached process-wide.
func findInstalled(name string) *plugin.DiscoveredPlugin {
	plugins, _ := plugin.GetPlugins()

	for i := range plugins {
		if plugins[i].Manifest.Name == name {
			return &plugins[i]
		}
	}

	return nil
}

// installFromRegistry installs the named plugins from the remote plugin
// registry, fetching the registry index once for all of them.
func installFromRegistry(cmd *cobra.Command, names []string) error {
	var (
		registry *plugin.PluginRegistry
		baseURL  string
	)

	err := tui.RunWithSpinner(
		fmt.Sprintf("Fetching plugin registry from %s…", plugin.PluginRegistryURL),
		func() error {
			var fetchErr error

			registry, baseURL, fetchErr = plugin.FetchRegistry(plugin.PluginRegistryURL)

			return fetchErr
		},
	)
	if err != nil {
		return fmt.Errorf("failed to fetch plugin registry: %w", err)
	}

	for _, name := range names {
		if err := installPlugin(cmd, registry, baseURL, name); err != nil {
			return err
		}
	}

	return nil
}

func installPlugin(cmd *cobra.Command, registry *plugin.PluginRegistry, baseURL, name string) error {
	entry, ok := registry.Plugins[name]
	if !ok {
		return fmt.Errorf("plugin %q not found in the plugin registry", name)
	}

	version, err := plugin.ResolveVersion(entry.Versions, "latest")
	if err != nil {
		return fmt.Errorf("failed to resolve the latest version of plugin %q: %w", name, err)
	}

	if err := tui.RunWithSpinner(
		fmt.Sprintf("Installing %s %s…", entry.Name, version.Version),
		func() error { return plugin.InstallPlugin(entry, *version, baseURL) },
	); err != nil {
		return fmt.Errorf("failed to install plugin %q: %w", name, err)
	}

	fmt.Println(tui.SuccessStyle.Render("✓ Installed " + entry.Name + " " + version.Version))

	return checkAndInstallPluginDeps(cmd, name)
}

// checkAndInstallPluginDeps checks the freshly installed plugin's
// prerequisites (its versions.yaml) and offers to install what is missing.
// A declined prompt is not an error: the plugin itself installed fine.
func checkAndInstallPluginDeps(cmd *cobra.Command, pluginName string) error {
	confirm := func() bool {
		if cli.IsNonInteractive(cmd) {
			return true
		}

		fmt.Fprint(os.Stdout, "\nInstall missing dependencies? [Y/n]: ")

		return reader.AskYesNo()
	}

	if err := plugin.CheckAndInstallDeps(pluginName, confirm, os.Stdout); err != nil {
		if errors.Is(err, plugin.ErrDepsDeclined) {
			return nil
		}

		return err
	}

	return nil
}
