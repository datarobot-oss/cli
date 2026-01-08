// Copyright 2025 DataRobot, Inc. and its affiliates.
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

package completion

import (
	"bytes"
	"strings"
	"testing"

	internalShell "github.com/datarobot/cli/internal/shell"
	"github.com/spf13/cobra"
)

func TestSupportedShells(t *testing.T) {
	shells := internalShell.SupportedShells()

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
	cmd := Cmd()

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

	cmd := Cmd()
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
