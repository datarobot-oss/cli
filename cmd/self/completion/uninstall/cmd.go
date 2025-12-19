// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package uninstall

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/fsutil"
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
		return showUninstallDryRunMessage(shell)
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

func showUninstallDryRunMessage(shell string) error {
	fmt.Println(infoStyle.Render("🔍 Dry-run mode (no changes will be made)"))
	fmt.Println()
	fmt.Println("To proceed with uninstallation, run:")
	fmt.Println(infoStyle.Render("  " + version.CliName + " completion uninstall " + shell + " --yes"))

	return nil
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

func getUninstallPaths(shell internalShell.Shell) []string {
	switch shell {
	case internalShell.Zsh:
		return []string{
			filepath.Join(os.Getenv("HOME"), ".oh-my-zsh", "custom", "completions", "_"+version.CliName),
			filepath.Join(os.Getenv("HOME"), ".zsh", "completions", "_"+version.CliName),
		}
	case internalShell.Bash:
		return []string{
			filepath.Join(os.Getenv("HOME"), ".bash_completions", version.CliName),
		}
	case internalShell.Fish:
		return []string{
			filepath.Join(os.Getenv("HOME"), ".config", "fish", "completions", version.CliName+".fish"),
		}
	case internalShell.PowerShell:
		var paths []string

		if runtime.GOOS == "windows" {
			documentsPath := os.Getenv("USERPROFILE")
			if documentsPath == "" {
				documentsPath = os.Getenv("HOME")
			}

			documentsPath = filepath.Join(documentsPath, "Documents")

			// PowerShell Core
			paths = append(paths, filepath.Join(documentsPath, "PowerShell", "Microsoft.PowerShell_profile.ps1"))
			// Windows PowerShell
			paths = append(paths, filepath.Join(documentsPath, "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1"))
		} else {
			homeDir := os.Getenv("HOME")
			paths = append(paths, filepath.Join(homeDir, ".config", "powershell", "Microsoft.PowerShell_profile.ps1"))
		}

		return paths
	default:
		return []string{}
	}
}

func promptForUninstallConfirmation() (bool, error) {
	fmt.Print("Proceed with uninstallation? [y/N]: ")

	var response string

	_, err := fmt.Scanln(&response)
	if err != nil && err.Error() != "unexpected newline" {
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

func uninstallZsh() bool {
	var removed bool

	// Oh-My-Zsh location
	path1 := filepath.Join(os.Getenv("HOME"), ".oh-my-zsh", "custom", "completions", "_"+version.CliName)
	if fsutil.FileExists(path1) {
		_ = os.Remove(path1)
		fmt.Printf("%s Removed: %s\n", successStyle.Render("✓"), path1)

		removed = true
	}

	// Standard Zsh location
	path2 := filepath.Join(os.Getenv("HOME"), ".zsh", "completions", "_"+version.CliName)
	if fsutil.FileExists(path2) {
		_ = os.Remove(path2)
		fmt.Printf("%s Removed: %s\n", successStyle.Render("✓"), path2)

		removed = true
	}

	// Clear cache
	if removed {
		cachePattern := filepath.Join(os.Getenv("HOME"), ".zcompdump*")

		matches, _ := filepath.Glob(cachePattern)
		for _, match := range matches {
			_ = os.Remove(match)
		}
	}

	return removed
}

func uninstallBash() bool {
	path := filepath.Join(os.Getenv("HOME"), ".bash_completions", version.CliName)
	if fsutil.FileExists(path) {
		_ = os.Remove(path)
		fmt.Printf("%s Removed: %s\n", successStyle.Render("✓"), path)

		return true
	}

	return false
}

func uninstallFish() bool {
	path := filepath.Join(os.Getenv("HOME"), ".config", "fish", "completions", version.CliName+".fish")
	if fsutil.FileExists(path) {
		_ = os.Remove(path)
		fmt.Printf("%s Removed: %s\n", successStyle.Render("✓"), path)

		return true
	}

	return false
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
		if strings.Contains(line, "# "+version.CliName+" completion") {
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
