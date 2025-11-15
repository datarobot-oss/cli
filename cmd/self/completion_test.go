// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package self

import (
	"bytes"
	"strings"
	"testing"

	internalShell "github.com/datarobot/cli/internal/shell"
	"github.com/spf13/cobra"
)

func TestSupportedShells(t *testing.T) {
	shells := supportedShells()

	expected := []string{"bash", "zsh", "fish", "powershell"}

	if len(shells) != len(expected) {
		t.Errorf("expected %d shells, got %d", len(expected), len(shells))
	}

	for i, shell := range expected {
		if shells[i] != shell {
			t.Errorf("expected shell %s at index %d, got %s", shell, i, shells[i])
		}
	}
}

func TestCmd(t *testing.T) {
	cmd := CompletionCmd()

	if cmd == nil {
		t.Fatal("Cmd() returned nil")

		return
	}

	if cmd.Use != "completion [bash|zsh|fish|powershell]" {
		t.Errorf("unexpected Use: %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "completion") {
		t.Errorf("Short description should contain 'completion': %s", cmd.Short)
	}

	// Check that subcommands are added
	subcommands := cmd.Commands()

	foundInstall := false

	foundUninstall := false

	for _, subcmd := range subcommands {
		if subcmd.Name() == "install" {
			foundInstall = true
		}

		if subcmd.Name() == "uninstall" {
			foundUninstall = true
		}
	}

	if !foundInstall {
		t.Error("install subcommand not found")
	}

	if !foundUninstall {
		t.Error("uninstall subcommand not found")
	}
}

func TestCompletionGeneration(t *testing.T) {
	tests := []struct {
		name         string
		shell        internalShell.Shell
		expectedText string
	}{
		{
			name:         "bash completion",
			shell:        internalShell.Bash,
			expectedText: "__start_dr",
		},
		{
			name:         "zsh completion",
			shell:        internalShell.Zsh,
			expectedText: "#compdef",
		},
		{
			name:         "fish completion",
			shell:        internalShell.Fish,
			expectedText: "complete -c dr",
		},
		{
			name:         "powershell completion",
			shell:        internalShell.PowerShell,
			expectedText: "Register-ArgumentCompleter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootCmd := &cobra.Command{
				Use:   "dr",
				Short: "DataRobot CLI.",
			}

			var buf bytes.Buffer

			// Generate completion directly
			var err error

			switch tt.shell {
			case internalShell.Bash:
				err = rootCmd.GenBashCompletion(&buf)
			case internalShell.Zsh:
				err = rootCmd.GenZshCompletion(&buf)
			case internalShell.Fish:
				err = rootCmd.GenFishCompletion(&buf, true)
			case internalShell.PowerShell:
				err = rootCmd.GenPowerShellCompletionWithDesc(&buf)
			}

			if err != nil {
				t.Fatalf("failed to generate completion: %v", err)
			}

			output := buf.String()

			if !strings.Contains(output, tt.expectedText) {
				t.Errorf("expected output to contain %q, got output length: %d", tt.expectedText, len(output))
			}
		})
	}
}

func TestCompletionInvalidShell(t *testing.T) {
	rootCmd := &cobra.Command{
		Use:   "dr",
		Short: "DataRobot CLI.",
	}

	cmd := CompletionCmd()
	rootCmd.AddCommand(cmd)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	// Try invalid shell
	rootCmd.SetArgs([]string{"completion", "invalid-shell"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid shell, got nil")
	}
}
