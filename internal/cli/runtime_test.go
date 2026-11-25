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

package cli

import (
	"testing"

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsNonInteractiveEnv(t *testing.T) {
	t.Cleanup(viperx.Reset)
	t.Setenv(reader.NonInteractiveEnv, "1")

	assert.True(t, IsNonInteractive(nil))
}

func TestIsNonInteractiveFlag(t *testing.T) {
	t.Cleanup(viperx.Reset)
	t.Setenv(reader.NonInteractiveEnv, "")

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().BoolP(YesFlagName, "y", false, "skip prompts")
	require.NoError(t, cmd.ParseFlags([]string{"--" + YesFlagName}))

	assert.True(t, IsNonInteractive(cmd))
}

func TestIsNonInteractiveForceInteractiveDoesNotOverride(t *testing.T) {
	t.Cleanup(viperx.Reset)
	t.Setenv(reader.NonInteractiveEnv, "true")

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool(YesFlagName, false, "skip prompts")
	require.NoError(t, cmd.ParseFlags([]string{"--" + YesFlagName}))

	viperx.Set("force-interactive", true)

	// --force-interactive only forces setup wizards to re-run; it must not
	// veto --yes / non-interactive consent for confirmation prompts.
	assert.True(t, IsNonInteractive(cmd))
}

func TestSuppressNotices(t *testing.T) {
	tests := []struct {
		name           string
		nonInteractive string
		viperFormat    string
		args           []string
		want           bool
	}{
		{name: "interactive text run keeps the notices", want: false},
		{name: "unrelated args keep the notices", args: []string{"run", "--file", "out.json"}, want: false},
		{name: "--output-format json", args: []string{"--output-format", "json"}, want: true},
		{name: "--output-format=json", args: []string{"--output-format=json"}, want: true},
		{name: "--output-format text", args: []string{"--output-format", "text"}, want: false},
		{name: "json from the environment or config", viperFormat: "json", want: true},
		{name: "text from the environment or config", viperFormat: "text", want: false},
		{
			name:        "explicit text beats a json config value",
			viperFormat: "json",
			args:        []string{"--output-format", "text"},
			want:        false,
		},
		{name: "non-interactive env var", nonInteractive: "1", want: true},
		{
			name:           "non-interactive env var with explicit text output",
			nonInteractive: "true",
			args:           []string{"--output-format", "text"},
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viperx.Reset()
			t.Cleanup(viperx.Reset)
			t.Setenv(reader.NonInteractiveEnv, tt.nonInteractive)

			if tt.viperFormat != "" {
				viperx.Set("output-format", tt.viperFormat)
			}

			cmd := &cobra.Command{Use: "plugin", DisableFlagParsing: true}

			assert.Equal(t, tt.want, SuppressNotices(cmd, tt.args))
		})
	}
}

// A plugin command never parses its own flags, but cobra still parses the root's
// persistent flags that come before the plugin name (TraverseChildren), so
// `dr --output-format json <plugin>` must suppress notices too.
func TestSuppressNotices_RootFlagBeforePluginName(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want bool
	}{
		{"json before the plugin name", []string{"--output-format", "json", "plug", "run"}, true},
		{"text before the plugin name", []string{"--output-format", "text", "plug", "run"}, false},
		{"json after the plugin name", []string{"plug", "run", "--output-format", "json"}, true},
		{"no flag at all", []string{"plug", "run"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viperx.Reset()
			t.Cleanup(viperx.Reset)
			t.Setenv(reader.NonInteractiveEnv, "")

			var (
				format outputformat.OutputFormat
				got    bool
				ran    bool
			)

			root := &cobra.Command{Use: "dr", TraverseChildren: true}
			outputformat.AddPersistentFlag(root, &format)
			require.NoError(t, viperx.BindPFlag("output-format", root.PersistentFlags().Lookup("output-format")))

			root.AddCommand(&cobra.Command{
				Use:                "plug",
				DisableFlagParsing: true,
				Run: func(cmd *cobra.Command, args []string) {
					ran = true
					got = SuppressNotices(cmd, args)
				},
			})

			root.SetArgs(tt.argv)
			require.NoError(t, root.Execute())

			require.True(t, ran, "the plugin command must run")
			assert.Equal(t, tt.want, got)
		})
	}
}
