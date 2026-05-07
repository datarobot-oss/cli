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

package uninstall

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/fsutil"
	"github.com/datarobot/cli/internal/misc/reader"
	internalShell "github.com/datarobot/cli/internal/shell"
	"github.com/datarobot/cli/internal/version"
	"github.com/spf13/cobra"
)

var (
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	infoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
)

func Cmd() *cobra.Command {
	var yes bool

	var dryRun bool

	cmd := &cobra.Command{
		Use:   "uninstall [shell]",
		Short: "Uninstall shell completions.",
		Long: `Uninstall shell completions by detecting your shell and removing them from the standard location.

This command will:
- Detect your current shell (or use specified shell)
- Show what will be removed
- Ask for confirmation (unless '--yes' is specified)
- Remove completion files

By default, runs in preview mode. Use '--yes' to uninstall directly.`,
		Example: `  # Preview what would be removed (default behavior)
  ` + version.CliName + ` completion uninstall

  # Uninstall completions for your current shell
  ` + version.CliName + ` completion uninstall --yes

  # Uninstall completions for a specific shell
  ` + version.CliName + ` completion uninstall bash --yes
  ` + version.CliName + ` completion uninstall zsh --yes`,
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: internalShell.SupportedShells(),
		RunE: func(cmd *cobra.Command, args []string) error {
			var shell string
			if len(args) > 0 {
				shell = args[0]
			}

			// Determine if we're in dry-run mode
			// If '--yes' is specified, disable dry-run (unless '--dry-run=true' was explicitly set)
			effectiveDryRun := dryRun
			if yes && !cmd.Flags().Changed("dry-run") {
				effectiveDryRun = false
			} else if !yes && !cmd.Flags().Changed("dry-run") {
				// Default to dry-run if '--yes' is not specified
				effectiveDryRun = true
			}

			return runUninstall(shell, yes, effectiveDryRun)
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Automatically confirm uninstallation without prompting")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview mode: show what would be removed without making changes")

	return cmd
}

func runUninstall(specifiedShell string, yes, dryRun bool) error {
	shell, err := resolveShellForUninstall(specifiedShell)
	if err != nil {
		return err
	}

	fmt.Println()

	existingPaths := findExistingCompletions(internalShell.Shell(shell))
	if len(existingPaths) == 0 {
		fmt.Printf("%s No completion found.\n", infoStyle.Render("ℹ"))

		return nil
	}

	showUninstallationPlan(shell, existingPaths)

	// Dry-run mode
	if dryRun {
		showUninstallDryRunMessage(shell)
		return nil
	}

	// Ask for confirmation if not auto-confirmed
	if !yes {
		confirmed, err := promptForUninstallConfirmation()
		if err != nil {
			return err
		}

		if !confirmed {
			return nil
		}
	}

	fmt.Println()

	return performUninstall(internalShell.Shell(shell))
}

func resolveShellForUninstall(specifiedShell string) (string, error) {
	if specifiedShell != "" {
		fmt.Printf("%s Uninstalling for shell: %s\n", infoStyle.Render("→"), specifiedShell)

		return specifiedShell, nil
	}

	shell, err := internalShell.DetectShell()
	if err != nil {
		return "", err
	}

	fmt.Printf("%s Detected shell: %s\n", infoStyle.Render("→"), shell)

	return shell, nil
}

func findExistingCompletions(shell internalShell.Shell) []string {
	paths := getUninstallPaths(shell)

	var existingPaths []string

	for _, path := range paths {
		if fsutil.FileExists(path) {
			existingPaths = append(existingPaths, path)
		}
	}

	return existingPaths
}

func showUninstallationPlan(shell string, existingPaths []string) {
	fmt.Println(infoStyle.Render("Uninstallation Plan:"))
	fmt.Printf("  Shell:        %s\n", shell)
	fmt.Println("  Remove:")

	for _, path := range existingPaths {
		fmt.Printf("    - %s\n", path)
	}

	fmt.Println()
}

func showUninstallDryRunMessage(shell string) {
	fmt.Println(infoStyle.Render("🔍 Dry-run mode (no changes will be made)"))
	fmt.Println()
	fmt.Println("To proceed with uninstallation, run:")
	fmt.Println(infoStyle.Render("  " + version.CliName + " completion uninstall " + shell + " --yes"))
}

func performUninstall(shell internalShell.Shell) error {
	var removed bool

	switch shell {
	case internalShell.Zsh:
		removed = uninstallZsh()
	case internalShell.Bash:
		removed = uninstallBash()
	case internalShell.Fish:
		removed = uninstallFish()
	case internalShell.PowerShell:
		removed = uninstallPowerShell()
	default:
		return fmt.Errorf("Unsupported shell: %s.", shell)
	}

	if removed {
		fmt.Printf("%s Completion uninstalled.\n", successStyle.Render("✓"))
		fmt.Println()
		fmt.Println("Restart your shell to apply changes.")
	}

	return nil
}

func zshCompletionPaths(homeDir string) []string {
	dirs := []string{
		filepath.Join(homeDir, ".oh-my-zsh", "custom", "completions"),
		filepath.Join(homeDir, ".zsh", "completions"),
	}

	names := make([]string, 1+len(version.CliAliases))
	names[0] = "_" + version.CliName

	for i, alias := range version.CliAliases {
		names[i+1] = "_" + alias
	}

	paths := make([]string, 0, len(names)*len(dirs))

	for _, dir := range dirs {
		for _, name := range names {
			paths = append(paths, filepath.Join(dir, name))
		}
	}

	return paths
}

func fishCompletionPaths(homeDir string) []string {
	base := filepath.Join(homeDir, ".config", "fish", "completions")

	paths := make([]string, 1+len(version.CliAliases))
	paths[0] = filepath.Join(base, version.CliName+".fish")

	for i, alias := range version.CliAliases {
		paths[i+1] = filepath.Join(base, alias+".fish")
	}

	return paths
}

func getUninstallPaths(shell internalShell.Shell) []string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = ""
	}

	switch shell {
	case internalShell.Zsh:
		return zshCompletionPaths(homeDir)
	case internalShell.Bash:
		return []string{
			filepath.Join(homeDir, ".bash_completions", version.CliName),
		}
	case internalShell.Fish:
		return fishCompletionPaths(homeDir)
	case internalShell.PowerShell:
		var paths []string

		if runtime.GOOS == "windows" {
			documentsPath, err := os.UserHomeDir()
			if err != nil {
				return []string{}
			}

			documentsPath = filepath.Join(documentsPath, "Documents")

			// PowerShell Core
			paths = append(paths, filepath.Join(documentsPath, "PowerShell", "Microsoft.PowerShell_profile.ps1"))
			// Windows PowerShell
			paths = append(paths, filepath.Join(documentsPath, "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1"))
		} else {
			paths = append(paths, filepath.Join(homeDir, ".config", "powershell", "Microsoft.PowerShell_profile.ps1"))
		}

		return paths
	default:
		return []string{}
	}
}

func promptForUninstallConfirmation() (bool, error) {
	fmt.Print("Proceed with uninstallation? [y/N]: ")

	response, err := reader.ReadString()
	if err != nil {
		return false, fmt.Errorf("Failed to read input: %w", err)
	}

	response = strings.ToLower(strings.TrimSpace(response))
	if response != "y" && response != "yes" {
		fmt.Println()
		fmt.Println(infoStyle.Render("Uninstallation cancelled."))

		return false, nil
	}

	return true, nil
}

func removeFile(path string) bool {
	if !fsutil.FileExists(path) {
		return false
	}

	_ = os.Remove(path)
	fmt.Printf("%s Removed: %s\n", successStyle.Render("✓"), path)

	return true
}

func uninstallZsh() bool {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = ""
	}

	var removed bool

	for _, path := range zshCompletionPaths(homeDir) {
		if removeFile(path) {
			removed = true
		}
	}

	if removed {
		cachePattern := filepath.Join(homeDir, ".zcompdump*")

		matches, _ := filepath.Glob(cachePattern)
		for _, match := range matches {
			_ = os.Remove(match)
		}
	}

	return removed
}

func uninstallBash() bool {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = ""
	}

	return removeFile(filepath.Join(homeDir, ".bash_completions", version.CliName))
}

func uninstallFish() bool {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = ""
	}

	var removed bool

	for _, path := range fishCompletionPaths(homeDir) {
		if removeFile(path) {
			removed = true
		}
	}

	return removed
}

func uninstallPowerShell() bool {
	var removed bool

	paths := getUninstallPaths(internalShell.PowerShell)

	for _, profilePath := range paths {
		if removePowerShellCompletionFromProfile(profilePath) {
			removed = true
		}
	}

	return removed
}

func removePowerShellCompletionFromProfile(profilePath string) bool {
	if !fsutil.FileExists(profilePath) {
		return false
	}

	content, err := os.ReadFile(profilePath)
	if err != nil {
		return false
	}

	// Check if completion is configured
	if !strings.Contains(string(content), version.CliName+" completion powershell") {
		return false
	}

	// Remove completion section
	newContent := removeCompletionSection(string(content))

	// Write back
	if err := os.WriteFile(profilePath, []byte(newContent), 0o644); err != nil {
		fmt.Printf("%s Failed to update: %s\n", warnStyle.Render("⚠"), profilePath)

		return false
	}

	fmt.Printf("%s Removed completion from: %s\n", successStyle.Render("✓"), profilePath)

	return true
}

func isCompletionMarker(line string) bool {
	if strings.Contains(line, "# "+version.CliName+" completion") {
		return true
	}

	for _, alias := range version.CliAliases {
		if strings.Contains(line, "# "+alias+" alias completion") {
			return true
		}
	}

	return false
}

func removeCompletionSection(content string) string {
	lines := strings.Split(content, "\n")
	newLines := make([]string, 0, len(lines))

	skipNext := 0
	for _, line := range lines {
		if skipNext > 0 {
			skipNext--

			continue
		}

		// Look for the completion comment
		if isCompletionMarker(line) {
			// Skip this line and the next 3 lines (the if block)
			skipNext = 3

			// Also skip preceding blank line if present
			if len(newLines) > 0 && strings.TrimSpace(newLines[len(newLines)-1]) == "" {
				newLines = newLines[:len(newLines)-1]
			}

			continue
		}

		newLines = append(newLines, line)
	}

	return strings.Join(newLines, "\n")
}
