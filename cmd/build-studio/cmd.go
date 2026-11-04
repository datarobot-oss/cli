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

package buildstudio

import (
	"errors"
	"fmt"
	"os"

	plugininstall "github.com/datarobot/cli/cmd/plugin/install"
	"github.com/datarobot/cli/cmd/plugin/shared"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/features"
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
)

// Test seams: unit tests swap these out so they never run plugin discovery,
// touch the network, or install anything. Discovery in particular is cached
// process-wide behind a sync.Once, which test order must not consume.
var (
	getPlugins          = plugin.GetPlugins
	loadManagedPlugin   = plugin.LoadManagedPlugin
	installPluginByName = installViaPluginInstall
	executePlugin       = plugin.ExecutePlugin
	checkPluginUpdate   = shared.CheckAndPromptPluginUpdate
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "build-studio",
		GroupID: "core",
		Short:   "🎨 Open Build Studio (the dr xp Assist tab)",
		Long: `Open Build Studio — the Assist tab of the dr xp experimentation dashboard.

When the xp or opencode plugin is missing, it is installed from the plugin
registry first. All flags except -y/--yes and -h/--help are passed through
to dr xp.`,
		Example: `  dr build-studio
  dr build-studio --port 9000`,
		Args: cobra.ArbitraryArgs,
		// Unrecognized flags belong to dr xp (e.g. --port), so cobra must not
		// reject them; parseArgs extracts this command's own flags instead.
		DisableFlagParsing: true,
		RunE:               run,
	}

	// Registered for help output only: with flag parsing disabled, cobra
	// never parses it — parseArgs consumes it from the raw args instead.
	cmd.Flags().BoolP(cli.YesFlagName, "y", false, `Assume "yes" when prompted to install plugin dependencies.`)

	features.SetGate(cmd, "build-studio")

	telemetry.Track(cmd)

	return cmd
}

func run(cmd *cobra.Command, rawArgs []string) error {
	args := parseArgs(rawArgs)

	if args.help {
		return cmd.Help()
	}

	exitCode, err := runStudio(cmd, args)
	if err != nil {
		return err
	}

	// ExitCommand enqueues the command's telemetry event before exiting:
	// os.Exit skips the OnFinalize hook that normally fires it. Error paths
	// return above and reach the hook as usual.
	telemetry.ExitCommand(cmd, rawArgs, exitCode)

	return nil
}

// studioArgs is the result of splitting raw args into the flags this command
// owns and everything that is forwarded to dr xp.
type studioArgs struct {
	help        bool
	yes         bool
	passthrough []string
}

// consent reports whether dependency-install prompts are pre-approved,
// either by the --yes flag or by the non-interactive env var.
func (a studioArgs) consent() bool {
	return a.yes || reader.IsNonInteractive()
}

// parseArgs scans raw args manually (flag parsing is disabled so unknown
// flags reach dr xp): -y/--yes and -h/--help are consumed here, everything
// else is forwarded verbatim. A "--" ends extraction, so flags after it
// always reach dr xp untouched.
func parseArgs(rawArgs []string) studioArgs {
	var (
		parsed     studioArgs
		extracting = true
	)

	for _, arg := range rawArgs {
		if extracting {
			switch arg {
			case "--":
				extracting = false
			case "-y", "--yes":
				parsed.yes = true

				continue
			case "-h", "--help":
				parsed.help = true

				continue
			}
		}

		parsed.passthrough = append(parsed.passthrough, arg)
	}

	return parsed
}

// runStudio makes sure the plugins are present, offers pending updates, and
// launches xp, whose Assist tab is enabled by default since 2.3.0 (the
// pre-launch update prompt moves older installations forward). Returns xp's
// exit code.
func runStudio(cmd *cobra.Command, args studioArgs) (int, error) {
	xp, openCode, err := ensureStudioPlugins(cmd, args.consent())
	if err != nil {
		return 0, err
	}

	for _, p := range []*plugin.DiscoveredPlugin{xp, openCode} {
		checkPluginUpdate(p.Manifest.Name, p.Manifest.Version, p.Executable)
	}

	// The banner goes to stderr so stdout stays reserved for xp's own output
	// (e.g. under --output-format json).
	fmt.Fprintln(os.Stderr, tui.InfoStyle.Render("🔌 Running plugin: "+xpPluginName))
	log.Debug("Executing plugin", "name", xpPluginName, "executable", xp.Executable)

	var execOpts []plugin.ExecOption

	if args.consent() {
		execOpts = append(execOpts, plugin.WithDepsConsent())
	}

	return executePlugin(cmd.Context(), xp.Manifest, xp.Executable, args.passthrough, cmd.Root().PersistentFlags(), execOpts...), nil
}

// ensureStudioPlugins makes sure both plugins Build Studio needs are
// installed and returns them. Only xp gets executed; opencode is installed
// because xp drives it to power the Assist tab.
func ensureStudioPlugins(cmd *cobra.Command, consent bool) (xp, openCode *plugin.DiscoveredPlugin, err error) {
	xp, err = ensurePlugin(cmd, xpPluginName, consent)
	if err != nil {
		return nil, nil, err
	}

	openCode, err = ensurePlugin(cmd, openCodePluginName, consent)
	if err != nil {
		return nil, nil, err
	}

	return xp, openCode, nil
}

// ensurePlugin resolves an installed plugin, installing it first when missing.
func ensurePlugin(cmd *cobra.Command, name string, consent bool) (*plugin.DiscoveredPlugin, error) {
	p, err := resolveInstalled(name)
	if err != nil {
		return nil, err
	}

	if p != nil {
		return p, nil
	}

	if err := installPluginByName(cmd, name, consent); err != nil {
		return nil, fmt.Errorf(
			"install plugin %q: %w (if the registry is unreachable, install from an archive: dr plugin install %s --file <archive>)",
			name, err, name)
	}

	p, err = loadManagedPlugin(name)
	if err != nil {
		return nil, fmt.Errorf("failed to load plugin %q after installation: %w", name, err)
	}

	if p == nil {
		return nil, fmt.Errorf("plugin %q is not available after installation", name)
	}

	return p, nil
}

// installViaPluginInstall delegates to `dr plugin install <name>` in-process,
// so Build Studio reuses the full registry flow — URL normalization, version
// resolution, user messaging, and the dependency check — instead of
// duplicating it.
func installViaPluginInstall(cmd *cobra.Command, name string, consent bool) error {
	installCmd := plugininstall.Cmd()
	installCmd.SetContext(cmd.Context())

	if consent {
		if err := installCmd.Flags().Set(cli.YesFlagName, "true"); err != nil {
			return err
		}
	}

	return installCmd.RunE(installCmd, []string{name})
}

// resolveInstalled returns the installed plugin with the given name, nil when
// it is not installed, or an error when its state cannot be determined. It
// consults cached discovery results first (which also cover PATH-based
// plugins) and falls back to reading the managed plugins directory directly,
// so an installed plugin is still found when discovery is disabled or timed
// out. A corrupt managed installation resolves to nil — reinstalling repairs
// it — while transient failures (e.g. permission errors) abort instead of
// triggering a reinstall that would wipe the plugin directory.
func resolveInstalled(name string) (*plugin.DiscoveredPlugin, error) {
	if p := findInstalled(name); p != nil {
		return p, nil
	}

	p, err := loadManagedPlugin(name)
	if err != nil {
		if errors.Is(err, plugin.ErrCorruptInstall) {
			log.Debug("Managed plugin is corrupt, will reinstall", "plugin", name, "error", err)

			return nil, nil
		}

		return nil, fmt.Errorf("failed to inspect installed plugin %q: %w", name, err)
	}

	return p, nil
}

// findInstalled returns the discovered plugin with the given name, or nil
// when it is not installed. Discovery results are cached process-wide.
func findInstalled(name string) *plugin.DiscoveredPlugin {
	plugins, _ := getPlugins()

	for i := range plugins {
		if plugins[i].Manifest.Name == name {
			return &plugins[i]
		}
	}

	return nil
}
