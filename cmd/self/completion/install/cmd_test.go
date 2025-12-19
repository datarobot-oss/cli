// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package install

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/fsutil"
	internalShell "github.com/datarobot/cli/internal/shell"
	"github.com/spf13/cobra"
)

func TestDetectShell(t *testing.T) {
	originalShell := os.Getenv("SHELL")
	defer os.Setenv("SHELL", originalShell)

	tests := []struct {
		name        string
		shellEnv    string
		goos        string
		expected    string
		expectError bool
	}{
		{
			name:     "bash from SHELL env",
			shellEnv: "/bin/bash",
			expected: "bash",
		},
		{
			name:     "zsh from SHELL env",
			shellEnv: "/usr/local/bin/zsh",
			expected: "zsh",
		},
		{
			name:     "fish from SHELL env",
			shellEnv: "/usr/bin/fish",
			expected: "fish",
		},
		{
			name:        "no SHELL env on non-windows",
			shellEnv:    "",
			goos:        "linux",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("SHELL", tt.shellEnv)

			// Skip Windows-specific test if not on Windows
			if tt.goos == "windows" && runtime.GOOS != "windows" {
				t.Skip("Skipping Windows-specific test")
			}

			shell, err := internalShell.DetectShell()

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if shell != tt.expected {
				t.Errorf("expected shell %q, got %q", tt.expected, shell)
			}
		})
	}
}

func TestFileExists(t *testing.T) {
	// Create temporary file
	tmpFile, err := os.CreateTemp("", "test-file-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	defer os.Remove(tmpFile.Name())

	tmpFile.Close()

	if !fsutil.FileExists(tmpFile.Name()) {
		t.Error("fileExists returned false for existing file")
	}

	if fsutil.FileExists("/nonexistent/file/path") {
		t.Error("fileExists returned true for nonexistent file")
	}
}

func TestDirExists(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "test-dir-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	defer os.RemoveAll(tmpDir)

	if !fsutil.DirExists(tmpDir) {
		t.Error("dirExists returned false for existing directory")
	}

	if fsutil.DirExists("/nonexistent/directory/path") {
		t.Error("dirExists returned true for nonexistent directory")
	}

	// Test with a file (should return false)
	tmpFile, err := os.CreateTemp("", "test-file-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	defer os.Remove(tmpFile.Name())

	tmpFile.Close()

	if fsutil.DirExists(tmpFile.Name()) {
		t.Error("dirExists returned true for a file")
	}
}

func TestGetInstallFunc(t *testing.T) {
	rootCmd := &cobra.Command{
		Use:   "dr",
		Short: "DataRobot CLI.",
	}

	tests := []struct {
		name        string
		shell       internalShell.Shell
		force       bool
		expectError bool
		errorText   string
	}{
		{
			name:  "bash install",
			shell: internalShell.Bash,
			force: false,
		},
		{
			name:  "zsh install",
			shell: internalShell.Zsh,
			force: false,
		},
		{
			name:  "fish install",
			shell: internalShell.Fish,
			force: false,
		},
		{
			name:        "powershell install",
			shell:       internalShell.PowerShell,
			force:       false,
			expectError: false,
		},
		{
			name:        "invalid shell",
			shell:       internalShell.Shell("invalid"),
			force:       false,
			expectError: true,
			errorText:   "Unsupported shell",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, fn, err := getInstallFunc(rootCmd, tt.shell, tt.force)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errorText != "" && !contains(err.Error(), tt.errorText) {
					t.Errorf("expected error to contain %q, got %q", tt.errorText, err.Error())
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if path == "" {
				t.Error("expected non-empty path")
			}

			if fn == nil {
				t.Error("expected non-nil install function")
			}
		})
	}
}

func TestInstallZsh(t *testing.T) {
	rootCmd := &cobra.Command{
		Use:   "dr",
		Short: "DataRobot CLI.",
	}

	path, fn := installZsh(rootCmd, false)

	if path == "" {
		t.Error("expected non-empty install path")
	}

	if fn == nil {
		t.Error("expected non-nil install function")
	}

	// Check that path contains expected patterns
	if !contains(path, "zsh") && !contains(path, "_dr") {
		t.Errorf("expected path to contain zsh or _dr, got: %s", path)
	}
}

func TestInstallBash(t *testing.T) {
	rootCmd := &cobra.Command{
		Use:   "dr",
		Short: "DataRobot CLI.",
	}

	path, fn := installBash(rootCmd, false)

	if path == "" {
		t.Error("expected non-empty install path")
	}

	if fn == nil {
		t.Error("expected non-nil install function")
	}

	// Check that path contains expected patterns
	if !contains(path, "bash") && !contains(path, "dr") {
		t.Errorf("expected path to contain bash or dr, got: %s", path)
	}
}

func TestInstallFish(t *testing.T) {
	rootCmd := &cobra.Command{
		Use:   "dr",
		Short: "DataRobot CLI.",
	}

	path, fn := installFish(rootCmd, false)

	if path == "" {
		t.Error("expected non-empty install path")
	}

	if fn == nil {
		t.Error("expected non-nil install function")
	}

	// Check that path contains expected patterns
	if !contains(path, "fish") && !contains(path, "dr.fish") {
		t.Errorf("expected path to contain fish or dr.fish, got: %s", path)
	}
}

func TestInstallCmd(t *testing.T) {
	cmd := Cmd()

	if cmd == nil {
		t.Fatal("Cmd() returned nil")

		return
	}

	if cmd.Use != "install [shell]" {
		t.Errorf("unexpected Use: %s", cmd.Use)
	}

	// Check flags
	if cmd.Flags().Lookup("force") == nil {
		t.Error("force flag not found")
	}

	if cmd.Flags().Lookup("yes") == nil {
		t.Error("yes flag not found")
	}

	if cmd.Flags().Lookup("dry-run") == nil {
		t.Error("dry-run flag not found")
	}
}

func TestIsBashCompletionAvailable(_ *testing.T) {
	// This test just ensures the function doesn't panic
	// The actual result depends on the system
	result := isBashCompletionAvailable()

	// Just verify it returns a boolean (it always will, but this exercises the code)
	_ = result
}

func TestResolveShell(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "specified bash",
			input:    "bash",
			expected: "bash",
		},
		{
			name:     "specified zsh",
			input:    "zsh",
			expected: "zsh",
		},
		{
			name:     "specified fish",
			input:    "fish",
			expected: "fish",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shell, err := internalShell.ResolveShell(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if shell != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, shell)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
