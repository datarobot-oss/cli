// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package self

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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

	if !fileExists(tmpFile.Name()) {
		t.Error("fileExists returned false for existing file")
	}

	if fileExists("/nonexistent/file/path") {
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

	if !dirExists(tmpDir) {
		t.Error("dirExists returned false for existing directory")
	}

	if dirExists("/nonexistent/directory/path") {
		t.Error("dirExists returned true for nonexistent directory")
	}

	// Test with a file (should return false)
	tmpFile, err := os.CreateTemp("", "test-file-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	defer os.Remove(tmpFile.Name())

	tmpFile.Close()

	if dirExists(tmpFile.Name()) {
		t.Error("dirExists returned true for a file")
	}
}

func TestGetInstallFunc(t *testing.T) {
	rootCmd := &cobra.Command{
		Use:   "dr",
		Short: "DataRobot CLI",
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
			name:        "powershell not supported",
			shell:       internalShell.PowerShell,
			force:       false,
			expectError: true,
			errorText:   "PowerShell",
		},
		{
			name:        "invalid shell",
			shell:       internalShell.Shell("invalid"),
			force:       false,
			expectError: true,
			errorText:   "unsupported shell",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, fn, err := getCompletionInstallFunc(rootCmd, tt.shell, tt.force)

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
		Short: "DataRobot CLI",
	}

	path, fn := installCompletionZsh(rootCmd, false)

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
		Short: "DataRobot CLI",
	}

	path, fn := installCompletionBash(rootCmd, false)

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
		Short: "DataRobot CLI",
	}

	path, fn := installCompletionFish(rootCmd, false)

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

func TestInstallCmd(t *testing.T) {
	cmd := installCompletionCmd()

	if cmd == nil {
		t.Fatal("installCmd() returned nil")
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

func TestUninstallCmd(t *testing.T) {
	cmd := uninstallCompletionCmd()

	if cmd == nil {
		t.Fatal("uninstallCmd() returned nil")
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
			name:          "powershell empty",
			shell:         internalShell.PowerShell,
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := getCompletionUninstallPaths(tt.shell)

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
			shell, err := resolveShellForCompletionUninstall(tt.input)
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
			name:        "powershell not supported",
			shell:       internalShell.PowerShell,
			expectError: true,
			errorText:   "PowerShell",
		},
		{
			name:        "invalid shell",
			shell:       internalShell.Shell("invalid"),
			expectError: true,
			errorText:   "unsupported shell",
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
			err := performCompletionUninstall(tt.shell)

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
	removed := uninstallCompletionZsh()

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
	removed = uninstallCompletionZsh()

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
	removed := uninstallCompletionBash()

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
	removed = uninstallCompletionBash()

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
	removed := uninstallCompletionFish()

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
	removed = uninstallCompletionFish()

	if !removed {
		t.Error("expected true when file exists")
	}

	if fileExists(compFile) {
		t.Error("completion file still exists after uninstall")
	}
}

// Helper function
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
