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
	"testing"

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- universalFlagEnv unit tests ---

func TestUniversalFlagEnv_AllUnset(t *testing.T) {
	viperx.Reset()

	result := universalFlagEnv()

	assert.Empty(t, result, "no env vars should be emitted when no universal flags are set")
}

func TestUniversalFlagEnv_DebugSet(t *testing.T) {
	viperx.Reset()
	viperx.Set("debug", true)

	result := universalFlagEnv()

	assert.Contains(t, result, "DATAROBOT_CLI_DEBUG=1")
	assert.NotContains(t, result, "DATAROBOT_CLI_DISABLE_TELEMETRY=1")
}

func TestUniversalFlagEnv_DisableTelemetrySet(t *testing.T) {
	viperx.Reset()
	viperx.Set("disable-telemetry", true)

	result := universalFlagEnv()

	assert.Contains(t, result, "DATAROBOT_CLI_DISABLE_TELEMETRY=1")
	assert.NotContains(t, result, "DATAROBOT_CLI_DEBUG=1")
}

func TestUniversalFlagEnv_BothSet(t *testing.T) {
	viperx.Reset()
	viperx.Set("debug", true)
	viperx.Set("disable-telemetry", true)

	result := universalFlagEnv()

	assert.Contains(t, result, "DATAROBOT_CLI_DEBUG=1")
	assert.Contains(t, result, "DATAROBOT_CLI_DISABLE_TELEMETRY=1")
}

func TestUniversalFlagEnv_BoolFalseOmitted(t *testing.T) {
	viperx.Reset()
	viperx.Set("debug", false)
	viperx.Set("disable-telemetry", false)

	result := universalFlagEnv()

	assert.Empty(t, result, "false bool flags must not be emitted")
}

// --- TraverseChildren / core-blind invariant tests ---

// buildTestTree returns an isolated cobra command tree that mirrors the real CLI
// wiring: root with TraverseChildren + persistent --debug, and a plugin-style
// child with DisableFlagParsing:true that records the args it receives.
func buildTestTree(t *testing.T) (root *cobra.Command, receivedArgs *[]string, debugSet *bool) {
	t.Helper()

	var got []string

	var dbg bool

	child := &cobra.Command{
		Use:                "plug",
		DisableFlagParsing: true,
		DisableSuggestions: true,
		Run: func(_ *cobra.Command, args []string) {
			got = args
		},
	}

	root = &cobra.Command{
		Use:              "dr",
		TraverseChildren: true,
		SilenceErrors:    true,
		SilenceUsage:     true,
	}
	root.PersistentFlags().Bool("debug", false, "debug output")

	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		dbg, _ = root.PersistentFlags().GetBool("debug")

		return nil
	}

	root.AddCommand(child)

	return root, &got, &dbg
}

// TestTraverseChildren_PrePluginFlagConsumedByCore verifies that --debug placed
// BEFORE the plugin name is parsed by core (debug set) and NOT forwarded to the
// plugin as a literal arg.
func TestTraverseChildren_PrePluginFlagConsumedByCore(t *testing.T) {
	root, receivedArgs, debugSet := buildTestTree(t)
	root.SetArgs([]string{"--debug", "plug", "foo", "bar"})

	err := root.Execute()
	require.NoError(t, err)

	assert.True(t, *debugSet, "core must see --debug when it appears before plugin name")
	assert.Equal(t, []string{"foo", "bar"}, *receivedArgs,
		"plugin must receive only its own args, not the consumed --debug flag")
}

// TestTraverseChildren_PostPluginFlagsInvisibleToCore is the hard invariant:
// core stays BLIND to any args after the plugin name (kubectl/helm model).
// --debug after the plugin name must NOT set core debug, and must pass through
// to the plugin verbatim.
func TestTraverseChildren_PostPluginFlagsInvisibleToCore(t *testing.T) {
	root, receivedArgs, debugSet := buildTestTree(t)
	root.SetArgs([]string{"plug", "--debug", "foo"})

	err := root.Execute()
	require.NoError(t, err)

	assert.False(t, *debugSet, "core must NOT see --debug when it appears after plugin name")
	assert.Equal(t, []string{"--debug", "foo"}, *receivedArgs,
		"plugin must receive --debug verbatim when it appears after the plugin name")
}
