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

package cmd

import (
	"bytes"
	"testing"

	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/tools"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCaseInsensitiveCommands(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		shouldError bool
	}{
		{
			name:        "HELP uppercase",
			args:        []string{"HELP"},
			shouldError: false,
		},
		{
			name:        "help lowercase",
			args:        []string{"help"},
			shouldError: false,
		},
		{
			name:        "Help mixed case",
			args:        []string{"Help"},
			shouldError: false,
		},
		{
			name:        "SELF uppercase",
			args:        []string{"SELF"},
			shouldError: false,
		},
		{
			name:        "self lowercase",
			args:        []string{"self"},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new root command for each test to ensure isolation
			cmd := RootCmd

			// Capture output
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			// Set the args
			cmd.SetArgs(tt.args)

			// Execute the command
			err := cmd.Execute()

			if tt.shouldError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestVersionFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "--version", args: []string{"--version"}},
		{name: "-V", args: []string{"-V"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// pflag does not reset flag values between Parse calls, so the
			// version flag stays true after this test. Reset it so subsequent
			// tests that execute the root command aren't affected.
			t.Cleanup(func() {
				_ = RootCmd.PersistentFlags().Set("version", "false")
			})

			cmd := RootCmd
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			require.NoError(t, err)

			output := buf.String()
			assert.NotEmpty(t, output, "Version output should not be empty")
		})
	}
}

// TestTelemetryPropExtractor_OnSuccessPath verifies that the DR CLI telemetry
// event is produced correctly on the success path. Uses dr dependency check with
// an echo-based tool (always satisfies the version check) so RunE returns nil.
func TestTelemetryPropExtractor_OnSuccessPath(t *testing.T) {
	orig := tools.RequiredTools

	defer func() { tools.RequiredTools = orig }()

	tools.RequiredTools = []tools.Prerequisite{
		{Name: "echo-tool", Command: "echo 1.0.0", MinimumVersion: "1.0.0"},
	}

	cmd := RootCmd
	cmd.SetArgs([]string{"dependency", "check"})

	var outBuf bytes.Buffer

	cmd.SetOut(&outBuf)

	err := cmd.Execute()
	require.NoError(t, err)

	checkCmd := findCommandByPath(RootCmd.Command, "dr dependency check")
	require.NotNil(t, checkCmd)

	event, ok := telemetry.EventFor(checkCmd, []string{})
	require.True(t, ok)
	assert.Equal(t, "dr dependency check", event.EventType)
	assert.Empty(t, event.EventProperties["missing_deps"])
	assert.Empty(t, event.EventProperties["wrong_version_deps"])
}

// TestTelemetryPropExtractor_OnErrorPath verifies that the DR CLI telemetry event
// is produced correctly on the error path. PersistentPostRunE (the previous approach)
// was skipped on error, silently dropping telemetry for failed commands.
func TestTelemetryPropExtractor_OnErrorPath(t *testing.T) {
	orig := tools.RequiredTools

	defer func() { tools.RequiredTools = orig }()

	tools.RequiredTools = []tools.Prerequisite{
		{Name: "FakeTool", Command: "nonexistent_dr_fake_xyz", URL: "https://example.com"},
	}

	cmd := RootCmd
	cmd.SetArgs([]string{"dependency", "check"})

	var errBuf bytes.Buffer

	cmd.SetErr(&errBuf)

	_ = cmd.Execute() // returns error: missing dep

	checkCmd := findCommandByPath(RootCmd.Command, "dr dependency check")
	require.NotNil(t, checkCmd)

	event, ok := telemetry.EventFor(checkCmd, []string{})
	require.True(t, ok)
	assert.Equal(t, "dr dependency check", event.EventType)

	missingMsgs, _ := event.EventProperties["missing_deps"].([]string)
	assert.NotEmpty(t, missingMsgs,
		"PropExtractor must see missing_deps populated by RunE even on the error path")
}

func TestUnknownArgGuard(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid subcommand passes",
			args:    []string{"self", "version"},
			wantErr: false,
		},
		{
			name:        "unknown subcommand on parent command errors",
			args:        []string{"self", "not-a-thing"},
			wantErr:     true,
			errContains: "unknown command: not-a-thing",
		},
		{
			name:    "parent command with no args shows help without error",
			args:    []string{"self"},
			wantErr: false,
		},
		{
			name:        "unknown top-level command errors",
			args:        []string{"not-a-command"},
			wantErr:     true,
			errContains: "unknown command: not-a-command",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := RootCmd

			var buf bytes.Buffer

			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSetUnknownArgGuards_AppliesGuardToPureParent(t *testing.T) {
	child := &cobra.Command{
		Use:  "child",
		RunE: func(_ *cobra.Command, _ []string) error { return nil },
	}
	parent := &cobra.Command{Use: "parent"}

	parent.AddCommand(child)

	setUnknownArgGuards(parent)

	require.NotNil(t, parent.Args, "pure parent command should have Args guard installed")
	require.NotNil(t, parent.RunE, "pure parent command should have RunE installed")

	err := parent.Args(parent, []string{"bogus"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command: bogus")
	assert.NoError(t, parent.Args(parent, []string{}))
}

func TestSetUnknownArgGuards_SkipsCommandWithRunE(t *testing.T) {
	child := &cobra.Command{
		Use:  "child",
		RunE: func(_ *cobra.Command, _ []string) error { return nil },
	}
	parent := &cobra.Command{
		Use:  "parent",
		RunE: func(_ *cobra.Command, _ []string) error { return nil },
	}

	parent.AddCommand(child)

	setUnknownArgGuards(parent)

	assert.Nil(t, parent.Args, "parent with RunE should not have Args guard installed")
}

// TestSetUnknownArgGuards_RootLevelUnknownCommand verifies that a root-like
// command (pure parent, no RunE) rejects an unrecognised first arg with the
// expected "unknown command" message.
//
// Uses a fresh, isolated command tree rather than the global RootCmd to avoid
// state pollution from cobra's package-level finalizers slice, which accumulates
// across Execute calls in other tests and can cause RootCmd to appear non-runnable
// by the time this assertion runs in the full test suite.
func TestSetUnknownArgGuards_RootLevelUnknownCommand(t *testing.T) {
	child := &cobra.Command{
		Use:  "child",
		RunE: func(_ *cobra.Command, _ []string) error { return nil },
	}
	root := &cobra.Command{Use: "root"}

	root.AddCommand(child)

	setUnknownArgGuards(root)

	require.NotNil(t, root.Args)

	err := root.Args(root, []string{"not-a-command"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command: not-a-command")
}

func TestSetUnknownArgGuards_SkipsExplicitArgs(t *testing.T) {
	child := &cobra.Command{
		Use:  "child",
		RunE: func(_ *cobra.Command, _ []string) error { return nil },
	}
	parent := &cobra.Command{
		Use:  "parent",
		Args: cobra.NoArgs,
	}

	parent.AddCommand(child)

	setUnknownArgGuards(parent)

	err := parent.Args(parent, []string{"x"})

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "unknown command:", "explicit Args validator should not be overridden")
}

func TestWorkloadCommandNotPresentByDefault(t *testing.T) {
	// Verify that workload command is not present by default (feature not enabled).
	// The feature gating happens during init(), so this tests the actual state.
	cmd := RootCmd

	var found bool

	for _, subCmd := range cmd.Commands() {
		if subCmd.Name() == "workload" {
			found = true
			break
		}
	}

	assert.False(t, found, "workload command should not be present when feature gate is not enabled")
}

func TestArtifactCommandNotPresentByDefault(t *testing.T) {
	// The artifact command shares the "workload" feature gate, so it is
	// filtered out by cli.CommandAdder during init() when the gate is not
	// enabled (the default).
	cmd := RootCmd

	var found bool

	for _, subCmd := range cmd.Commands() {
		if subCmd.Name() == "artifact" {
			found = true
			break
		}
	}

	assert.False(t, found, "artifact command should not be present when feature gate is not enabled")
}

// TestRootCmdTraverseChildrenEnabled is a guard that fails immediately if
// TraverseChildren is ever removed from RootCmd. Without it, universal flags
// placed before a plugin name (e.g. "dr --debug myplugin") would be silently
// swallowed into the plugin's raw args instead of being parsed by core.
func TestRootCmdTraverseChildrenEnabled(t *testing.T) {
	assert.True(t, RootCmd.TraverseChildren,
		"RootCmd must have TraverseChildren:true so universal flags (--debug, etc.) "+
			"placed before a plugin name are consumed by core and not forwarded as raw args")
}

// TestUniversalFlagsParsedOnCoreSubcommand verifies that a universal flag such as
// --debug is parsed correctly when it appears after a core subcommand and that
// subcommand's own flags (e.g. "dr <cmd> --set-url http://x --debug").
// This guards the TraverseChildren behaviour for core commands: unlike plugins,
// core subcommands have no DisableFlagParsing so cobra continues to parse flags
// after the command name, including persistent flags from root.
func TestUniversalFlagsParsedOnCoreSubcommand(t *testing.T) {
	var parsedDebug bool

	sentinel := &cobra.Command{
		Use:           "sentinel-universal-flags-test",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Read via the persistent flag on the root command.
			parsedDebug, _ = cmd.Root().PersistentFlags().GetBool("debug")

			return nil
		},
	}
	sentinel.Flags().String("set-url", "", "test flag")

	RootCmd.AddCommand(sentinel)

	defer func() {
		RootCmd.RemoveCommand(sentinel)
		// Reset the debug persistent flag so it does not bleed into other tests.
		_ = RootCmd.PersistentFlags().Set("debug", "false")
	}()

	RootCmd.SetArgs([]string{"sentinel-universal-flags-test", "--set-url", "http://example.com", "--debug"})

	err := RootCmd.Execute()
	require.NoError(t, err)

	assert.True(t, parsedDebug,
		"--debug must be parsed by core when it appears after a core subcommand and its own flags")
}
