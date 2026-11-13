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

package install

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/datarobot/cli/cmd/self/completion/powershell"
	"github.com/datarobot/cli/internal/fsutil"
	internalShell "github.com/datarobot/cli/internal/shell"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/datarobot/cli/internal/version"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectShell(t *testing.T) {
	// DetectShell() now prioritizes parent process detection over $SHELL.
	// In the test environment, the parent is the test runner (e.g., "go")
	// so we can't easily test the $SHELL fallback without process mocking.
	// This test verifies that DetectShell returns a non-empty result.
	shell, err := internalShell.DetectShell()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if shell == "" {
		t.Error("expected non-empty shell name")
	}

	// In test environments, parent process is usually the test runner
	t.Logf("Detected shell: %s (parent process of test runner)", shell)
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
			installer, err := getInstaller(rootCmd, tt.shell, tt.force)

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

			if installer.Path() == "" {
				t.Error("expected non-empty path")
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

// TestShowAlreadyInstalled guards against CFX-7958 command path drift.
func TestShowAlreadyInstalled(t *testing.T) {
	cmd := Cmd()
	testutil.WireCompletionCommandPath(cmd)

	out := testutil.CaptureStdout(t, func() {
		showAlreadyInstalled(cmd, "/fake/path/_dr")
	})

	if !strings.Contains(out, "/fake/path/_dr") {
		t.Errorf("output %q does not mention the install path", out)
	}

	wantHint := version.CliName + " self completion install --force --yes"
	if !strings.Contains(out, wantHint) {
		t.Errorf("output %q does not contain expected reinstall hint %q", out, wantHint)
	}
}

// TestShowDryRunMessage guards against CFX-7958 command path drift.
func TestShowDryRunMessage(t *testing.T) {
	cmd := Cmd()
	testutil.WireCompletionCommandPath(cmd)

	out := testutil.CaptureStdout(t, func() {
		showDryRunMessage(cmd, "zsh")
	})

	wantHint := version.CliName + " self completion install zsh --yes"
	if !strings.Contains(out, wantHint) {
		t.Errorf("output %q does not contain expected dry-run hint %q", out, wantHint)
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

func testEnsureFpathInZshrcHelper(t *testing.T, content, shouldContain string) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "test-zshrc-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	zshrcPath := filepath.Join(tmpDir, ".zshrc")
	if err := os.WriteFile(zshrcPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write zshrc: %v", err)
	}

	compDir := filepath.Join(tmpDir, ".zsh", "completions")
	if err := ensureFpathInZshrc(zshrcPath, compDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fileContent, err := os.ReadFile(zshrcPath)
	if err != nil {
		t.Fatalf("failed to read zshrc: %v", err)
	}

	if !strings.Contains(string(fileContent), shouldContain) {
		t.Errorf("expected content to contain %q, got: %s", shouldContain, string(fileContent))
	}
}

func TestEnsureFpathInZshrc(t *testing.T) {
	t.Run("add fpath to empty zshrc", func(t *testing.T) {
		testEnsureFpathInZshrcHelper(t, "", "fpath=")
	})
	t.Run("add fpath to existing content", func(t *testing.T) {
		testEnsureFpathInZshrcHelper(t, "export PATH=/usr/local/bin:$PATH\n", "fpath=")
	})
	t.Run("already contains fpath", func(t *testing.T) {
		testEnsureFpathInZshrcHelper(t, "fpath=(/custom/path $fpath)\n", "/custom/path")
	})
}

func TestEnsureFpathInZshrcNotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-zshrc-notfound-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	nonexistentPath := filepath.Join(tmpDir, "nonexistent", ".zshrc")
	compDir := filepath.Join(tmpDir, ".zsh", "completions")

	err = ensureFpathInZshrc(nonexistentPath, compDir)
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}

	if !strings.Contains(err.Error(), "~/.zshrc not found") {
		t.Errorf("expected error to mention ~/.zshrc not found, got: %v", err)
	}
}

func testEnsureSourceInBashrcHelper(t *testing.T, compFileInTmpDir, initialContent, expectedInFile string, expectIdempotent bool) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "test-bashrc-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	bashrcPath := filepath.Join(tmpDir, ".bashrc")
	compFile := filepath.Join(tmpDir, compFileInTmpDir)

	// Pre-populate bashrc with initial content if provided
	if expectIdempotent {
		// For idempotent test, include the source line that should already exist
		initialContent = fmt.Sprintf("[ -f %s ] && source %s\n", compFile, compFile)
	}

	if err := os.WriteFile(bashrcPath, []byte(initialContent), 0o644); err != nil {
		t.Fatalf("failed to write bashrc: %v", err)
	}

	if err := ensureSourceInBashrc(bashrcPath, compFile); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fileContent, err := os.ReadFile(bashrcPath)
	if err != nil {
		t.Fatalf("failed to read bashrc: %v", err)
	}

	contentStr := string(fileContent)
	if !strings.Contains(contentStr, expectedInFile) {
		t.Errorf("expected content to contain %q, got: %s", expectedInFile, contentStr)
	}

	// For idempotent test, verify it only appears once
	if expectIdempotent {
		sourceCount := strings.Count(contentStr, fmt.Sprintf("[ -f %s ]", compFile))
		if sourceCount != 1 {
			t.Errorf("expected source line to appear exactly once, got %d", sourceCount)
		}
	}
}

func TestEnsureSourceInBashrc(t *testing.T) {
	t.Run("add source to empty bashrc", func(t *testing.T) {
		testEnsureSourceInBashrcHelper(t, ".bash_completions/dr", "", "source", false)
	})
	t.Run("add source to bashrc with existing content", func(t *testing.T) {
		testEnsureSourceInBashrcHelper(t, ".bash_completions/dr", "export PATH=/usr/local/bin:$PATH\n", "source", false)
	})
	t.Run("already contains source line", func(t *testing.T) {
		testEnsureSourceInBashrcHelper(t, ".bash_completions/dr", "", "source", true)
	})
}

func TestEnsureSourceInBashrcNotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-bashrc-notfound-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	nonexistentPath := filepath.Join(tmpDir, "nonexistent", ".bashrc")
	completionFile := filepath.Join(tmpDir, ".bash_completions", "dr")

	err = ensureSourceInBashrc(nonexistentPath, completionFile)
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}

	if !strings.Contains(err.Error(), "~/.bashrc not found") {
		t.Errorf("expected error to mention ~/.bashrc not found, got: %v", err)
	}
}

const testCompletionCommand = "dr self completion powershell"

// powerShellInstall returns the profile path and the install function of the PowerShell installer.
func powerShellInstall(t *testing.T) (string, func(*cobra.Command) error) {
	t.Helper()

	installer, err := newPowerShellInstaller(testCompletionCommand)
	require.NoError(t, err)

	return installer.Path(), installer.Install
}

func TestInstallPowerShell(t *testing.T) {
	rootCmd := &cobra.Command{
		Use:   "dr",
		Short: "DataRobot CLI.",
	}

	tmpDir, err := os.MkdirTemp("", "test-powershell-profile-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testutil.SetTestHomeDir(t, tmpDir)

	profilePath, installFn := powerShellInstall(t)

	if profilePath == "" {
		t.Error("expected non-empty profile path")
	}

	if installFn == nil {
		t.Fatal("expected non-nil install function")
	}

	if err := installFn(rootCmd); err != nil {
		t.Fatalf("installPowerShell failed: %v", err)
	}

	if !fsutil.FileExists(profilePath) {
		t.Fatalf("expected profile to exist at %s", profilePath)
	}

	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("failed to read profile: %v", err)
	}

	content := string(data)

	if !contains(content, "dr self completion powershell") {
		t.Error("profile should contain dr completion block")
	}
}

// installOverLegacyProfile installs completion over a profile an earlier release wrote and returns
// the profile path and the install function.
func installOverLegacyProfile(t *testing.T) (string, func(*cobra.Command) error) {
	t.Helper()

	rootCmd := &cobra.Command{Use: "dr", Short: "DataRobot CLI."}

	testutil.SetTestHomeDir(t, t.TempDir())

	profilePath, installFn := powerShellInstall(t)

	if err := os.MkdirAll(filepath.Dir(profilePath), 0o755); err != nil {
		t.Fatalf("failed to create profile dir: %v", err)
	}

	legacy := "Set-Alias ll Get-ChildItem\n\n# dr completion\nif (Get-Command dr -ErrorAction SilentlyContinue) {\n    dr completion powershell | Out-String | Invoke-Expression\n}\n"
	if err := os.WriteFile(profilePath, []byte(legacy), 0o600); err != nil {
		t.Fatalf("failed to write profile: %v", err)
	}

	if err := installFn(rootCmd); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	return profilePath, installFn
}

func readProfile(t *testing.T, profilePath string) string {
	t.Helper()

	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("failed to read profile: %v", err)
	}

	return string(data)
}

func TestInstallPowerShellMigratesALegacyProfile(t *testing.T) {
	profilePath, _ := installOverLegacyProfile(t)

	got := readProfile(t, profilePath)

	if contains(got, "dr completion powershell") {
		t.Errorf("legacy command must be gone, got: %s", got)
	}

	if !contains(got, "Set-Alias ll Get-ChildItem") {
		t.Errorf("the user's own lines must stay, got: %s", got)
	}

	if strings.Count(got, "# dr completion\n") != 1 {
		t.Errorf("the dr block must not be duplicated, got: %s", got)
	}
}

func TestInstallPowerShellOverAMigratedProfileChangesNothing(t *testing.T) {
	profilePath, installFn := installOverLegacyProfile(t)

	first := readProfile(t, profilePath)

	if err := installFn(&cobra.Command{Use: "dr"}); err != nil {
		t.Fatalf("second install failed: %v", err)
	}

	if second := readProfile(t, profilePath); second != first {
		t.Errorf("a second install must not change the profile, got: %s", second)
	}
}

func TestPowerShellInstallerInstalled(t *testing.T) {
	legacy := "\n# dr completion\nif (Get-Command dr -ErrorAction SilentlyContinue) {\n    dr completion powershell | Out-String | Invoke-Expression\n}\n"
	onlyDr := "\n# dr completion\nif (Get-Command dr -ErrorAction SilentlyContinue) {\n    dr self completion powershell | Out-String | Invoke-Expression\n}\n"

	tests := []struct {
		name    string
		profile []byte // nil leaves no profile on disk
		want    bool
	}{
		{"no profile", nil, false},
		{"a profile without completion", []byte("Set-Alias ll Get-ChildItem\n"), false},
		{"a profile from an earlier release", []byte(legacy), false},
		{"a profile with only the dr block", []byte(onlyDr), false},
		{"a profile that is not UTF-8", []byte{0xFF, 0xFE, '#', 0}, false},
		{"a profile with every current block", []byte(onlyDr + powershell.MissingBlocks(onlyDr, testCompletionCommand)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.SetTestHomeDir(t, t.TempDir())

			installer, err := newPowerShellInstaller(testCompletionCommand)
			require.NoError(t, err)

			if tt.profile != nil {
				require.NoError(t, os.MkdirAll(filepath.Dir(installer.Path()), 0o755))
				require.NoError(t, os.WriteFile(installer.Path(), tt.profile, 0o600))
			}

			assert.Equal(t, tt.want, installer.Installed())
		})
	}
}

func TestPowerShellInstallerIsInstalledAfterInstall(t *testing.T) {
	testutil.SetTestHomeDir(t, t.TempDir())

	installer, err := newPowerShellInstaller(testCompletionCommand)
	require.NoError(t, err)

	require.NoError(t, installer.Install(&cobra.Command{Use: "dr"}))

	assert.True(t, installer.Installed())
}

// wiredInstallCmd returns an install command inside the shipped command tree.
func wiredInstallCmd() *cobra.Command {
	cmd := Cmd()
	testutil.WireCompletionCommandPath(cmd)

	return cmd
}

func TestInstallForShellMigratesALegacyProfileWithoutForce(t *testing.T) {
	testutil.SetTestHomeDir(t, t.TempDir())

	profilePath, _ := powerShellInstall(t)
	legacy := "Set-Alias ll Get-ChildItem\n\n# dr completion\nif (Get-Command dr -ErrorAction SilentlyContinue) {\n    dr completion powershell | Out-String | Invoke-Expression\n}\n"

	require.NoError(t, os.MkdirAll(filepath.Dir(profilePath), 0o755))
	require.NoError(t, os.WriteFile(profilePath, []byte(legacy), 0o600))

	cmd := wiredInstallCmd()

	testutil.CaptureStdout(t, func() {
		require.NoError(t, installForShell(cmd, "powershell", internalShell.PowerShell, false, true, false))
	})

	migrated := readProfile(t, profilePath)

	assert.NotContains(t, migrated, "dr completion powershell")
	assert.Contains(t, migrated, "dr self completion powershell")

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, installForShell(cmd, "powershell", internalShell.PowerShell, false, true, false))
	})

	assert.Contains(t, out, "already installed", "a migrated profile counts as installed")
	assert.Equal(t, migrated, readProfile(t, profilePath), "a second install changes nothing")
}

func TestInstallForShellBuildsTheProfileLineFromTheCommandTree(t *testing.T) {
	testutil.SetTestHomeDir(t, t.TempDir())

	root := &cobra.Command{Use: "tool"}
	completion := &cobra.Command{Use: "completion"}
	cmd := Cmd()

	completion.AddCommand(cmd)
	root.AddCommand(completion)

	testutil.CaptureStdout(t, func() {
		require.NoError(t, installForShell(cmd, "powershell", internalShell.PowerShell, false, true, false))
	})

	profilePath, _ := powerShellInstall(t)
	got := readProfile(t, profilePath)

	assert.Contains(t, got, "    tool completion powershell | Out-String | Invoke-Expression\n")
	assert.NotContains(t, got, "dr self completion powershell")
}

// installOverProfile writes a profile into a fresh home, installs PowerShell completion over it and
// returns the profile path and the install error.
func installOverProfile(t *testing.T, content []byte) (string, error) {
	t.Helper()

	rootCmd := &cobra.Command{Use: "dr", Short: "DataRobot CLI."}

	testutil.SetTestHomeDir(t, t.TempDir())

	profilePath, installFn := powerShellInstall(t)

	require.NoError(t, os.MkdirAll(filepath.Dir(profilePath), 0o755))
	require.NoError(t, os.WriteFile(profilePath, content, 0o600))

	return profilePath, installFn(rootCmd)
}

// hasBareLineFeed reports whether text holds a line feed that no carriage return precedes.
func hasBareLineFeed(text string) bool {
	return strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\n")
}

func TestInstallPowerShellKeepsCRLFLineEndings(t *testing.T) {
	t.Run("appended blocks use CRLF", func(t *testing.T) {
		profilePath, err := installOverProfile(t, []byte("Set-Alias ll Get-ChildItem\r\n"))
		require.NoError(t, err)

		got := readProfile(t, profilePath)

		assert.Contains(t, got, "dr self completion powershell")
		assert.False(t, hasBareLineFeed(got), "the profile holds a bare line feed: %q", got)
	})

	t.Run("a migrated profile keeps CRLF", func(t *testing.T) {
		legacy := "Set-Alias ll Get-ChildItem\r\n\r\n# dr completion\r\nif (Get-Command dr -ErrorAction SilentlyContinue) {\r\n    dr completion powershell | Out-String | Invoke-Expression\r\n}\r\n"

		profilePath, err := installOverProfile(t, []byte(legacy))
		require.NoError(t, err)

		got := readProfile(t, profilePath)

		assert.NotContains(t, got, "dr completion powershell")
		assert.Contains(t, got, "# datarobot alias completion", "the alias block the profile lacked is added")
		assert.False(t, hasBareLineFeed(got), "the profile holds a bare line feed: %q", got)
	})
}

func TestInstallPowerShellRefusesAProfileItCannotEdit(t *testing.T) {
	utf16 := []byte{0xFF, 0xFE, 'S', 0, 'e', 0, 't', 0, '-', 0, 'A', 0, 'l', 0, 'i', 0, 'a', 0, 's', 0, '\r', 0, '\n', 0}

	profilePath, err := installOverProfile(t, utf16)

	require.ErrorIs(t, err, powershell.ErrNotUTF8)
	assert.Equal(t, string(utf16), readProfile(t, profilePath), "a refused profile must stay as it was")
}

func TestInstallPowerShellMigratesOnlyTheBlockItWrote(t *testing.T) {
	existing := "mydr completion powershell\n# run dr completion powershell by hand\n\n# dr completion\nif (Get-Command dr -ErrorAction SilentlyContinue) {\n    dr completion powershell | Out-String | Invoke-Expression\n}\n"

	profilePath, err := installOverProfile(t, []byte(existing))
	require.NoError(t, err)

	got := readProfile(t, profilePath)

	assert.Contains(t, got, "mydr completion powershell\n", "a similar command the user wrote must stay")
	assert.Contains(t, got, "# run dr completion powershell by hand\n", "a comment the user wrote must stay")
	assert.Contains(t, got, "    dr self completion powershell | Out-String | Invoke-Expression\n")
}

func TestInstallPowerShellKeepsTheProfileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX file modes")
	}

	profilePath, _ := installOverLegacyProfile(t)

	info, err := os.Stat(profilePath)
	require.NoError(t, err)

	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestInstallPowerShellWritesThroughASymlinkedProfile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating a symlink needs a privilege on Windows")
	}

	rootCmd := &cobra.Command{Use: "dr", Short: "DataRobot CLI."}

	testutil.SetTestHomeDir(t, t.TempDir())

	profilePath, installFn := powerShellInstall(t)
	target := filepath.Join(t.TempDir(), "dotfiles-profile.ps1")

	require.NoError(t, os.MkdirAll(filepath.Dir(profilePath), 0o755))
	require.NoError(t, os.WriteFile(target, []byte("\n# dr completion\nif (Get-Command dr -ErrorAction SilentlyContinue) {\n    dr completion powershell | Out-String | Invoke-Expression\n}\n"), 0o600))
	require.NoError(t, os.Symlink(target, profilePath))

	require.NoError(t, installFn(rootCmd))

	info, err := os.Lstat(profilePath)
	require.NoError(t, err)

	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the profile must stay a symlink")
	assert.Contains(t, readProfile(t, target), "dr self completion powershell", "the link target holds the migrated profile")
}

func TestMigrateProfiles(t *testing.T) {
	dir := t.TempDir()
	legacy := "\n# dr completion\nif (Get-Command dr -ErrorAction SilentlyContinue) {\n    dr completion powershell | Out-String | Invoke-Expression\n}\n"
	current := strings.ReplaceAll(legacy, "dr completion powershell", "dr self completion powershell")
	utf16 := string([]byte{0xFF, 0xFE, '#', 0, ' ', 0})

	write := func(name, content string) string {
		path := filepath.Join(dir, name)

		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

		return path
	}

	installed := write("installed.ps1", legacy)
	other := write("other.ps1", legacy)
	already := write("already.ps1", current)
	refused := write("utf16.ps1", utf16)
	missing := filepath.Join(dir, "missing.ps1")

	changed, err := migrateProfiles([]string{installed, other, already, refused, missing}, installed, testCompletionCommand)

	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, current, readProfile(t, other), "the other profile is migrated")
	assert.Equal(t, legacy, readProfile(t, installed), "the installed profile is left to its own update")
	assert.Equal(t, current, readProfile(t, already), "a current profile stays")
	assert.Equal(t, utf16, readProfile(t, refused), "a UTF-16 profile is skipped, not corrupted")
	assert.NoFileExists(t, missing)

	changed, err = migrateProfiles([]string{other, already, missing}, installed, testCompletionCommand)

	require.NoError(t, err)
	assert.False(t, changed, "a second pass changes nothing")
}

// Helper function.
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
