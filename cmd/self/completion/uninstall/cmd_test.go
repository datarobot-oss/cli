// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package uninstall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	internalShell "github.com/datarobot/cli/internal/shell"
)

func TestFindExistingCompletions(t *testing.T) {
	// Create temporary directory structure
	tmpDir, err := os.MkdirTemp("", "test-completions-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	defer os.RemoveAll(tmpDir)

	// Save original HOME
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	// Set HOME to temp directory
	os.Setenv("HOME", tmpDir)

	tests := []struct {
		name          string
		shell         internalShell.Shell
		setupFiles    []string
		expectedCount int
	}{
		{
			name:  "zsh - oh-my-zsh completion",
			shell: internalShell.Zsh,
			setupFiles: []string{
				filepath.Join(tmpDir, ".oh-my-zsh", "custom", "completions", "_dr"),
			},
			expectedCount: 1,
		},
		{
			name:  "bash completion",
			shell: internalShell.Bash,
			setupFiles: []string{
				filepath.Join(tmpDir, ".bash_completions", "dr"),
			},
			expectedCount: 1,
		},
		{
			name:  "fish completion",
			shell: internalShell.Fish,
			setupFiles: []string{
				filepath.Join(tmpDir, ".config", "fish", "completions", "dr.fish"),
			},
			expectedCount: 1,
		},
		{
			name:          "no completions",
			shell:         internalShell.Zsh,
			setupFiles:    []string{},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up temp directory
			os.RemoveAll(tmpDir)

			if err := os.MkdirAll(tmpDir, 0o755); err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}

			os.Setenv("HOME", tmpDir)

			// Create test files
			for _, filePath := range tt.setupFiles {
				dir := filepath.Dir(filePath)

				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("failed to create directory: %v", err)
				}

				if err := os.WriteFile(filePath, []byte("test"), 0o644); err != nil {
					t.Fatalf("failed to create file: %v", err)
				}
			}

			paths := findExistingCompletions(tt.shell)

			if len(paths) != tt.expectedCount {
				t.Errorf("expected %d paths, got %d: %v", tt.expectedCount, len(paths), paths)
			}
		})
	}
}

func TestUninstallCmd(t *testing.T) {
	cmd := Cmd()

	if cmd == nil {
		t.Fatal("Cmd() returned nil")

		return
	}

	if cmd.Use != "uninstall [shell]" {
		t.Errorf("unexpected Use: %s", cmd.Use)
	}

	// Check flags
	if cmd.Flags().Lookup("yes") == nil {
		t.Error("yes flag not found")
	}

	if cmd.Flags().Lookup("dry-run") == nil {
		t.Error("dry-run flag not found")
	}
}

func TestGetUninstallPaths(t *testing.T) {
	// Save original HOME
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	testHome := "/test/home"
	os.Setenv("HOME", testHome)

	tests := []struct {
		name          string
		shell         internalShell.Shell
		expectedCount int
		checkPath     string
	}{
		{
			name:          "zsh paths",
			shell:         internalShell.Zsh,
			expectedCount: 2,
			checkPath:     ".oh-my-zsh",
		},
		{
			name:          "bash paths",
			shell:         internalShell.Bash,
			expectedCount: 1,
			checkPath:     ".bash_completions",
		},
		{
			name:          "fish paths",
			shell:         internalShell.Fish,
			expectedCount: 1,
			checkPath:     ".config/fish",
		},
		{
			name:          "powershell paths",
			shell:         internalShell.PowerShell,
			expectedCount: 1,
			checkPath:     "PowerShell",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := getUninstallPaths(tt.shell)

			if len(paths) != tt.expectedCount {
				t.Errorf("expected %d paths, got %d", tt.expectedCount, len(paths))
			}

			if tt.checkPath != "" && tt.expectedCount > 0 {
				found := false

				for _, path := range paths {
					if strings.Contains(path, tt.checkPath) {
						found = true

						break
					}
				}

				if !found {
					t.Errorf("expected at least one path to contain %q", tt.checkPath)
				}
			}
		})
	}
}

func TestResolveShellForUninstall(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shell, err := resolveShellForUninstall(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if shell != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, shell)
			}
		})
	}
}

func TestPerformUninstall(t *testing.T) {
	tests := []struct {
		name        string
		shell       internalShell.Shell
		expectError bool
		errorText   string
	}{
		{
			name:  "zsh uninstall",
			shell: internalShell.Zsh,
		},
		{
			name:  "bash uninstall",
			shell: internalShell.Bash,
		},
		{
			name:  "fish uninstall",
			shell: internalShell.Fish,
		},
		{
			name:        "powershell uninstall",
			shell:       internalShell.PowerShell,
			expectError: false,
		},
		{
			name:        "invalid shell",
			shell:       internalShell.Shell("invalid"),
			expectError: true,
			errorText:   "Unsupported shell",
		},
	}

	// Create temp home
	tmpDir, err := os.MkdirTemp("", "test-uninstall-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	os.Setenv("HOME", tmpDir)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := performUninstall(tt.shell)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errorText != "" && !strings.Contains(err.Error(), tt.errorText) {
					t.Errorf("expected error to contain %q, got %q", tt.errorText, err.Error())
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestUninstallZsh(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-uninstall-zsh-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	os.Setenv("HOME", tmpDir)

	// Test with no files
	removed := uninstallZsh()

	if removed {
		t.Error("expected false when no files exist")
	}

	// Create completion file
	compDir := filepath.Join(tmpDir, ".oh-my-zsh", "custom", "completions")

	if err := os.MkdirAll(compDir, 0o755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	compFile := filepath.Join(compDir, "_dr")

	if err := os.WriteFile(compFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// Test with file
	removed = uninstallZsh()

	if !removed {
		t.Error("expected true when file exists")
	}

	if fileExists(compFile) {
		t.Error("completion file still exists after uninstall")
	}
}

func TestUninstallBash(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-uninstall-bash-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	os.Setenv("HOME", tmpDir)

	// Test with no files
	removed := uninstallBash()

	if removed {
		t.Error("expected false when no files exist")
	}

	// Create completion file
	compDir := filepath.Join(tmpDir, ".bash_completions")

	if err := os.MkdirAll(compDir, 0o755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	compFile := filepath.Join(compDir, "dr")

	if err := os.WriteFile(compFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// Test with file
	removed = uninstallBash()

	if !removed {
		t.Error("expected true when file exists")
	}

	if fileExists(compFile) {
		t.Error("completion file still exists after uninstall")
	}
}

func TestUninstallFish(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-uninstall-fish-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	os.Setenv("HOME", tmpDir)

	// Test with no files
	removed := uninstallFish()

	if removed {
		t.Error("expected false when no files exist")
	}

	// Create completion file
	compDir := filepath.Join(tmpDir, ".config", "fish", "completions")

	if err := os.MkdirAll(compDir, 0o755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	compFile := filepath.Join(compDir, "dr.fish")

	if err := os.WriteFile(compFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// Test with file
	removed = uninstallFish()

	if !removed {
		t.Error("expected true when file exists")
	}

	if fileExists(compFile) {
		t.Error("completion file still exists after uninstall")
	}
}
