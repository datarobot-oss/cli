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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/outputformat"
	internalPlugin "github.com/datarobot/cli/internal/plugin"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPluginDiscoveryTimeoutRoot(t *testing.T) *cobra.Command {
	t.Helper()

	root := &cobra.Command{Use: "dr"}
	root.PersistentFlags().Duration(
		internalPlugin.DiscoveryTimeoutKey,
		internalPlugin.DefaultDiscoveryTimeout,
		"",
	)

	return root
}

func TestPluginDiscoveryTimeout_FlagBeatsEnv(t *testing.T) {
	t.Setenv("DATAROBOT_CLI_PLUGIN_DISCOVERY_TIMEOUT", "9s")

	root := newPluginDiscoveryTimeoutRoot(t)
	require.NoError(t, root.PersistentFlags().Set(internalPlugin.DiscoveryTimeoutKey, "0s"))

	assert.Equal(t, time.Duration(0), pluginDiscoveryTimeout(root))
}

func TestPluginDiscoveryTimeout_UsesEnvBeforeConfigIsRead(t *testing.T) {
	t.Setenv("DATAROBOT_CLI_PLUGIN_DISCOVERY_TIMEOUT", "25ms")

	root := newPluginDiscoveryTimeoutRoot(t)

	assert.Equal(t, 25*time.Millisecond, pluginDiscoveryTimeout(root))
}

func TestPluginDiscoveryTimeout_InvalidEnvFallsBackToDefault(t *testing.T) {
	t.Setenv("DATAROBOT_CLI_PLUGIN_DISCOVERY_TIMEOUT", "not-a-duration")

	root := newPluginDiscoveryTimeoutRoot(t)

	assert.Equal(t, internalPlugin.DefaultDiscoveryTimeout, pluginDiscoveryTimeout(root))
}

// This test consumes the package-global plugin registry (PrimeCache/GetPlugins
// use sync.Once); no other test in this package reads that registry.
func TestRegisterPluginCommands_DisabledSeedsEmptyLazyCache(t *testing.T) {
	t.Setenv("DATAROBOT_CLI_PLUGIN_DISCOVERY_TIMEOUT", "9s")

	tempDir := t.TempDir()
	testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", tempDir)

	// A valid plugin on PATH: if the lazy path re-discovered despite the
	// startup flag, the 9s env value would find it and this test would fail.
	pluginPath := filepath.Join(tempDir, "dr-fakeplugin")
	script := "#!/bin/sh\necho '{\"name\":\"fakeplugin\",\"version\":\"1.0.0\"}'\n"
	require.NoError(t, os.WriteFile(pluginPath, []byte(script), 0o755))
	t.Setenv("PATH", tempDir)

	root := newPluginDiscoveryTimeoutRoot(t)
	require.NoError(t, root.PersistentFlags().Set(internalPlugin.DiscoveryTimeoutKey, "0s"))

	RegisterPluginCommands(root)

	plugins, conflicts := internalPlugin.GetPlugins()
	assert.Empty(t, plugins, "lazy discovery must not re-run after startup disabled it")
	assert.Empty(t, conflicts)
}

func TestOutputFormatFromArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantFormat string
		wantFound  bool
	}{
		{"no args", nil, "", false},
		{"unrelated args", []string{"run", "--verbose"}, "", false},
		{"separate value", []string{"--output-format", "json"}, "json", true},
		{"equals value", []string{"--output-format=json"}, "json", true},
		{"text value", []string{"--output-format", "text"}, "text", true},
		{"after other args", []string{"run", "app", "--output-format", "json"}, "json", true},
		{"last occurrence wins", []string{"--output-format", "json", "--output-format=text"}, "text", true},
		{"flag without a value", []string{"--output-format"}, "", false},
		{"stops at the terminator", []string{"--", "--output-format", "json"}, "", false},
		{"value that merely contains json", []string{"--file", "out.json"}, "", false},
		{"deprecated -o alias is not recognised", []string{"-o", "json"}, "", false},
		{"deprecated --format alias is not recognised", []string{"--format", "json"}, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format, found := outputFormatFromArgs(tt.args)

			assert.Equal(t, tt.wantFormat, format)
			assert.Equal(t, tt.wantFound, found)
		})
	}
}

// resetViperWithEnv starts the test from a clean viper that reads
// DATAROBOT_CLI_* env vars the way the real CLI does, and restores a clean
// viper afterwards.
func resetViperWithEnv(t *testing.T) {
	t.Helper()

	viperx.Reset()
	viperx.SetEnvPrefix("DATAROBOT_CLI")
	viperx.AutomaticEnv()
	viperx.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	t.Cleanup(viperx.Reset)
}

func TestSuppressBanner(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		args []string
		want bool
	}{
		{"interactive text run shows the banner", nil, nil, false},
		{"plugin args that are not output flags", nil, []string{"run", "--file", "out.json"}, false},
		{"--output-format json", nil, []string{"--output-format", "json"}, true},
		{"--output-format=json", nil, []string{"--output-format=json"}, true},
		{"--output-format text", nil, []string{"--output-format", "text"}, false},
		{
			"explicit text beats a json env var",
			map[string]string{"DATAROBOT_CLI_OUTPUT_FORMAT": "json"},
			[]string{"--output-format", "text"},
			false,
		},
		{"json env var", map[string]string{"DATAROBOT_CLI_OUTPUT_FORMAT": "json"}, nil, true},
		{"text env var", map[string]string{"DATAROBOT_CLI_OUTPUT_FORMAT": "text"}, nil, false},
		{"non-interactive env var", map[string]string{"DATAROBOT_CLI_NON_INTERACTIVE": "1"}, nil, true},
		{
			"non-interactive env var with text output",
			map[string]string{"DATAROBOT_CLI_NON_INTERACTIVE": "true"},
			[]string{"--output-format", "text"},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear the signals first so the host environment cannot leak in.
			t.Setenv("DATAROBOT_CLI_NON_INTERACTIVE", "")
			t.Setenv("DATAROBOT_CLI_OUTPUT_FORMAT", "")

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			resetViperWithEnv(t)

			var format outputformat.OutputFormat

			cmd := &cobra.Command{Use: "plugin", DisableFlagParsing: true}
			outputformat.AddPersistentFlag(cmd, &format)

			require.NoError(t, viperx.BindPFlag("output-format", cmd.PersistentFlags().Lookup("output-format")))

			assert.Equal(t, tt.want, suppressBanner(cmd, tt.args))
		})
	}
}
