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
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/datarobot/cli/internal/plugin"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubSeams replaces every seam with a safe default that fails the test when
// an unexpected dependency is reached, and restores the real implementations
// on cleanup. Tests override individual seams after calling it, so no test
// can ever run real discovery, hit the network, or install anything —
// regardless of the order the shuffler picks.
func stubSeams(t *testing.T) {
	t.Helper()

	origGetPlugins := getPlugins
	origLoadManaged := loadManagedPlugin
	origInstall := installPluginByName
	origExecute := executePlugin
	origUpdate := checkPluginUpdate
	origStderr := stderr

	t.Cleanup(func() {
		getPlugins = origGetPlugins
		loadManagedPlugin = origLoadManaged
		installPluginByName = origInstall
		executePlugin = origExecute
		checkPluginUpdate = origUpdate
		stderr = origStderr
	})

	getPlugins = func() ([]plugin.DiscoveredPlugin, []plugin.PluginConflict) {
		return nil, nil
	}
	loadManagedPlugin = func(string) (*plugin.DiscoveredPlugin, error) {
		return nil, nil
	}
	installPluginByName = func(_ *cobra.Command, name string, _ bool) error {
		t.Fatalf("unexpected install of plugin %q", name)

		return nil
	}
	executePlugin = func(_ context.Context, _ plugin.PluginManifest, _ string, _ []string, _ *pflag.FlagSet, _ ...plugin.ExecOption) int {
		t.Fatal("unexpected plugin execution")

		return 1
	}
	checkPluginUpdate = func(string, string, string) {}
	stderr = &bytes.Buffer{}
}

func discovered(name, executable string) plugin.DiscoveredPlugin {
	return plugin.DiscoveredPlugin{
		Manifest: plugin.PluginManifest{
			BasicPluginManifest: plugin.BasicPluginManifest{Name: name, Version: "1.0.0"},
		},
		Executable: executable,
	}
}

func TestCmdMetadata(t *testing.T) {
	cmd := Cmd()

	assert.Equal(t, "build-studio", cmd.Use)
	assert.Equal(t, "core", cmd.GroupID)
	assert.NotNil(t, cmd.RunE)
	assert.True(t, cmd.DisableFlagParsing, "unknown flags must reach dr xp, so cobra must not parse them")
	assert.NotNil(t, cmd.Flags().Lookup("yes"), "studio should document the --yes flag")
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name        string
		rawArgs     []string
		want        studioArgs
		passthrough []string
	}{
		{
			name:    "empty",
			rawArgs: nil,
			want:    studioArgs{},
		},
		{
			name:    "yes short and long are consumed",
			rawArgs: []string{"-y", "--yes"},
			want:    studioArgs{yes: true},
		},
		{
			name:        "unknown flags pass through",
			rawArgs:     []string{"--port", "9000"},
			want:        studioArgs{},
			passthrough: []string{"--port", "9000"},
		},
		{
			name:    "help is consumed",
			rawArgs: []string{"--help"},
			want:    studioArgs{help: true},
		},
		{
			name:        "flags after double dash are not extracted",
			rawArgs:     []string{"-y", "--", "-y", "--help"},
			want:        studioArgs{yes: true},
			passthrough: []string{"--", "-y", "--help"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseArgs(tt.rawArgs)

			assert.Equal(t, tt.want.yes, got.yes)
			assert.Equal(t, tt.want.help, got.help)
			assert.Equal(t, tt.passthrough, got.passthrough)
		})
	}
}

func TestRunStudio_AllInstalled(t *testing.T) {
	stubSeams(t)

	getPlugins = func() ([]plugin.DiscoveredPlugin, []plugin.PluginConflict) {
		return []plugin.DiscoveredPlugin{
			discovered("xp", "/fake/xp.sh"),
			discovered("opencode", "/fake/opencode.sh"),
		}, nil
	}

	var (
		gotExecutable string
		gotArgs       []string
		gotOptCount   int
		updateChecked []string
	)

	executePlugin = func(_ context.Context, _ plugin.PluginManifest, executable string, args []string, _ *pflag.FlagSet, opts ...plugin.ExecOption) int {
		gotExecutable = executable
		gotArgs = args
		gotOptCount = len(opts)

		return 7
	}
	checkPluginUpdate = func(name, _, _ string) {
		updateChecked = append(updateChecked, name)
	}

	exitCode, err := runStudio(Cmd(), studioArgs{passthrough: []string{"--port", "9000"}})

	require.NoError(t, err)
	assert.Equal(t, 7, exitCode, "xp's exit code should propagate")
	assert.Equal(t, "/fake/xp.sh", gotExecutable)
	assert.Equal(t, []string{"--port", "9000"}, gotArgs,
		"passthrough args must reach xp unchanged")
	assert.Zero(t, gotOptCount, "no consent option without --yes")
	assert.Equal(t, []string{"xp", "opencode"}, updateChecked, "both plugins should get an update check")
}

func TestRunStudio_YesGrantsDepsConsent(t *testing.T) {
	stubSeams(t)

	getPlugins = func() ([]plugin.DiscoveredPlugin, []plugin.PluginConflict) {
		return []plugin.DiscoveredPlugin{
			discovered("xp", "/fake/xp.sh"),
			discovered("opencode", "/fake/opencode.sh"),
		}, nil
	}

	var gotOptCount int

	executePlugin = func(_ context.Context, _ plugin.PluginManifest, _ string, _ []string, _ *pflag.FlagSet, opts ...plugin.ExecOption) int {
		gotOptCount = len(opts)

		return 0
	}

	_, err := runStudio(Cmd(), studioArgs{yes: true})

	require.NoError(t, err)
	assert.Equal(t, 1, gotOptCount, "--yes should pass the explicit deps-consent option")
}

func TestRunStudio_InstallsMissing(t *testing.T) {
	stubSeams(t)

	var installed []string

	installedSet := map[string]bool{}

	installPluginByName = func(_ *cobra.Command, name string, consent bool) error {
		installed = append(installed, fmt.Sprintf("%s consent=%t", name, consent))
		installedSet[name] = true

		return nil
	}
	// Resolves only what was actually installed, mirroring the real managed
	// lookup: before installation both plugins must read as missing.
	loadManagedPlugin = func(name string) (*plugin.DiscoveredPlugin, error) {
		if !installedSet[name] {
			return nil, nil
		}

		p := discovered(name, "/managed/"+name+".sh")

		return &p, nil
	}
	executePlugin = func(_ context.Context, _ plugin.PluginManifest, executable string, _ []string, _ *pflag.FlagSet, _ ...plugin.ExecOption) int {
		assert.Equal(t, "/managed/xp.sh", executable)

		return 0
	}

	exitCode, err := runStudio(Cmd(), studioArgs{yes: true})

	require.NoError(t, err)
	assert.Zero(t, exitCode)
	assert.Equal(t, []string{"xp consent=true", "opencode consent=true"}, installed,
		"both missing plugins should be installed with consent forwarded")
}

func TestRunStudio_InstallsOnlyOpenCode(t *testing.T) {
	stubSeams(t)

	getPlugins = func() ([]plugin.DiscoveredPlugin, []plugin.PluginConflict) {
		return []plugin.DiscoveredPlugin{discovered("xp", "/fake/xp.sh")}, nil
	}

	var installed []string

	installedSet := map[string]bool{}

	installPluginByName = func(_ *cobra.Command, name string, _ bool) error {
		installed = append(installed, name)
		installedSet[name] = true

		return nil
	}
	loadManagedPlugin = func(name string) (*plugin.DiscoveredPlugin, error) {
		if !installedSet[name] {
			return nil, nil
		}

		p := discovered(name, "/managed/"+name+".sh")

		return &p, nil
	}
	executePlugin = func(_ context.Context, _ plugin.PluginManifest, executable string, _ []string, _ *pflag.FlagSet, _ ...plugin.ExecOption) int {
		assert.Equal(t, "/fake/xp.sh", executable, "the already-installed xp should be reused")

		return 0
	}

	_, err := runStudio(Cmd(), studioArgs{})

	require.NoError(t, err)
	assert.Equal(t, []string{"opencode"}, installed, "only the missing opencode plugin should be installed")
}

func TestRunStudio_InstallFailureHasArchiveHint(t *testing.T) {
	stubSeams(t)

	installPluginByName = func(_ *cobra.Command, _ string, _ bool) error {
		return errors.New("registry unreachable")
	}

	_, err := runStudio(Cmd(), studioArgs{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "registry unreachable")
	assert.Contains(t, err.Error(), "dr plugin install xp --file",
		"the error should point air-gapped users at the archive install path")
}

// TestResolveInstalled_ManagedFallback verifies that a plugin absent from the
// discovery results is still resolved via the managed plugins lookup, so
// disabled discovery does not trigger a reinstall.
func TestResolveInstalled_ManagedFallback(t *testing.T) {
	stubSeams(t)

	loadManagedPlugin = func(name string) (*plugin.DiscoveredPlugin, error) {
		require.Equal(t, "xp", name)

		p := discovered("xp", "/managed/xp.sh")

		return &p, nil
	}

	p, err := resolveInstalled("xp")

	require.NoError(t, err)
	require.NotNil(t, p, "a managed plugin should resolve even when absent from discovery results")
	assert.Equal(t, "/managed/xp.sh", p.Executable)
}

// TestResolveInstalled_CorruptTriggersReinstall verifies that a corrupt
// managed installation reads as "not installed" so it gets repaired by a
// reinstall.
func TestResolveInstalled_CorruptTriggersReinstall(t *testing.T) {
	stubSeams(t)

	loadManagedPlugin = func(string) (*plugin.DiscoveredPlugin, error) {
		return nil, fmt.Errorf("%w: manifest is garbage", plugin.ErrCorruptInstall)
	}

	p, err := resolveInstalled("xp")

	require.NoError(t, err)
	assert.Nil(t, p, "a corrupt installation should resolve to nil so it is reinstalled")
}

// TestResolveInstalled_TransientErrorAborts verifies that a transient load
// failure (e.g. permission denied) aborts instead of triggering a reinstall
// that would wipe the plugin directory.
func TestResolveInstalled_TransientErrorAborts(t *testing.T) {
	stubSeams(t)

	loadManagedPlugin = func(string) (*plugin.DiscoveredPlugin, error) {
		return nil, errors.New("permission denied")
	}

	_, err := resolveInstalled("xp")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestXPVersionWarning(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    []string
		notWant []string
	}{
		{
			name:    "below Assist",
			version: "2.2.0",
			want:    []string{"xp 2.2.0", "Assist needs 2.3.0", "Analyze trace needs 2.5.0", "dr plugin update xp"},
		},
		{
			name:    "between Assist and Analyze trace",
			version: "2.4.9",
			want:    []string{"xp 2.4.9", "Analyze trace needs 2.5.0"},
			notWant: []string{"Assist needs"},
		},
		{
			name:    "pre-release of the minimum",
			version: "2.5.0-rc.1",
			want:    []string{"Analyze trace needs 2.5.0"},
		},
		{name: "at the minimum", version: "2.5.0"},
		{name: "above the minimum", version: "3.0.1"},
		{name: "unparseable", version: "dev"},
		{name: "empty", version: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := xpVersionWarning(tt.version)

			if len(tt.want) == 0 {
				assert.Empty(t, got)

				return
			}

			for _, w := range tt.want {
				assert.Contains(t, got, w)
			}

			for _, nw := range tt.notWant {
				assert.NotContains(t, got, nw)
			}
		})
	}
}

// studioWithXP stubs an installed xp at the given version plus opencode, and
// records xp's execution. It returns the stderr buffer and the args xp got.
func studioWithXP(t *testing.T, xpVersion string) (*bytes.Buffer, *[]string) {
	t.Helper()
	stubSeams(t)

	xp := discovered("xp", "/managed/xp.sh")
	xp.Manifest.Version = xpVersion

	getPlugins = func() ([]plugin.DiscoveredPlugin, []plugin.PluginConflict) {
		return []plugin.DiscoveredPlugin{xp, discovered("opencode", "/managed/opencode.sh")}, nil
	}

	gotArgs := new([]string)

	executePlugin = func(_ context.Context, _ plugin.PluginManifest, _ string, args []string, _ *pflag.FlagSet, _ ...plugin.ExecOption) int {
		*gotArgs = args

		return 0
	}

	buf := &bytes.Buffer{}
	stderr = buf

	return buf, gotArgs
}

func TestRunStudio_WarnsOnOldXPAndStillStarts(t *testing.T) {
	buf, gotArgs := studioWithXP(t, "2.3.1")

	exitCode, err := runStudio(Cmd(), studioArgs{passthrough: []string{"--port", "9000"}})

	require.NoError(t, err)
	assert.Zero(t, exitCode)
	assert.Contains(t, buf.String(), "xp 2.3.1 is older than Build Studio needs: Analyze trace needs 2.5.0")
	assert.Equal(t, []string{"--port", "9000"}, *gotArgs, "xp should still start with the same args")
}

func TestRunStudio_NoWarningForCurrentXP(t *testing.T) {
	buf, _ := studioWithXP(t, "2.5.0")

	_, err := runStudio(Cmd(), studioArgs{})

	require.NoError(t, err)
	assert.NotContains(t, buf.String(), "older than Build Studio needs")
}

// TestRunStudio_NoWarningAfterAcceptedUpdate verifies that the version is
// read again after the update check, so an xp updated in place doesn't
// warn about the version it replaced.
func TestRunStudio_NoWarningAfterAcceptedUpdate(t *testing.T) {
	buf, _ := studioWithXP(t, "2.2.0")

	updated := false

	checkPluginUpdate = func(name, _, _ string) {
		if name == "xp" {
			updated = true
		}
	}
	loadManagedPlugin = func(name string) (*plugin.DiscoveredPlugin, error) {
		if name != "xp" || !updated {
			return nil, nil
		}

		p := discovered("xp", "/managed/xp.sh")
		p.Manifest.Version = "2.5.0"

		return &p, nil
	}

	_, err := runStudio(Cmd(), studioArgs{})

	require.NoError(t, err)
	assert.NotContains(t, buf.String(), "older than Build Studio needs")
}

// TestRunStudio_IgnoresOtherManagedXP verifies that a managed xp at another
// path than the one about to run (e.g. xp found on PATH) doesn't decide the
// warning.
func TestRunStudio_IgnoresOtherManagedXP(t *testing.T) {
	buf, _ := studioWithXP(t, "2.2.0")

	loadManagedPlugin = func(string) (*plugin.DiscoveredPlugin, error) {
		p := discovered("xp", "/elsewhere/xp.sh")
		p.Manifest.Version = "2.5.0"

		return &p, nil
	}

	_, err := runStudio(Cmd(), studioArgs{})

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "xp 2.2.0 is older than Build Studio needs")
}
