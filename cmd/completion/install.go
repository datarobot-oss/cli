// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package completion

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/version"
	"github.com/spf13/cobra"
)

var (
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	infoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
)

func installCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install shell completions interactively",
		Long: `Install shell completions automatically by detecting your shell and
installing to the appropriate location.

This command will:
- Detect your current shell
- Install completions to the standard location
- Clear completion cache (if needed)
- Show instructions to activate completions`,
		Example: `  # Install completions for your current shell
  ` + version.CliName + ` completion install

  # Force reinstall even if already installed
  ` + version.CliName + ` completion install --force`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInstall(cmd.Root(), force)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force reinstall even if already installed")

	return cmd
}

func runInstall(rootCmd *cobra.Command, force bool) error {
	shell, err := detectShell()
	if err != nil {
		return err
	}

	fmt.Printf("%s Detected shell: %s\n", infoStyle.Render("→"), shell)
	fmt.Println()

	var installPath string

	var installFunc func(*cobra.Command) error

	switch Shell(shell) {
	case ShellZsh:
		installPath, installFunc = installZsh(rootCmd, force)
	case ShellBash:
		installPath, installFunc = installBash(rootCmd, force)
	case ShellFish:
		installPath, installFunc = installFish(rootCmd, force)
	default:
		return fmt.Errorf("unsupported shell: %s", shell)
	}

	// Check if already installed
	if !force && fileExists(installPath) {
		fmt.Printf("%s Completion already installed at: %s\n", successStyle.Render("✓"), installPath)
		fmt.Println()
		fmt.Println(infoStyle.Render("To reinstall, use: " + version.CliName + " completion install --force"))

		return nil
	}

	// Install
	if err := installFunc(rootCmd); err != nil {
		return fmt.Errorf("failed to install completions: %w", err)
	}

	fmt.Printf("%s Completion installed to: %s\n", successStyle.Render("✓"), installPath)
	fmt.Println()

	// Show activation instructions
	showActivationInstructions(Shell(shell))

	return nil
}

func detectShell() (string, error) {
	// Try SHELL environment variable first
	shellPath := os.Getenv("SHELL")
	if shellPath != "" {
		return filepath.Base(shellPath), nil
	}

	// On Windows, check for PowerShell
	if runtime.GOOS == "windows" {
		return "powershell", nil
	}

	return "", fmt.Errorf("could not detect shell. Please set SHELL environment variable")
}

func installZsh(rootCmd *cobra.Command, force bool) (string, func(*cobra.Command) error) {
	var installPath string

	var compDir string

	// Check for Oh-My-Zsh first (most common)
	if dirExists(filepath.Join(os.Getenv("HOME"), ".oh-my-zsh")) {
		compDir = filepath.Join(os.Getenv("HOME"), ".oh-my-zsh", "custom", "completions")
		installPath = filepath.Join(compDir, "_"+version.CliName)
	} else {
		// Use standard Zsh completion directory
		compDir = filepath.Join(os.Getenv("HOME"), ".zsh", "completions")
		installPath = filepath.Join(compDir, "_"+version.CliName)
	}

	installFunc := func(rootCmd *cobra.Command) error {
		// Create directory
		if err := os.MkdirAll(compDir, 0755); err != nil {
			return err
		}

		// Create completion file
		f, err := os.Create(installPath)
		if err != nil {
			return err
		}
		defer f.Close()

		// Generate completion
		if err := rootCmd.GenZshCompletion(f); err != nil {
			return err
		}

		// Clear cache
		cachePattern := filepath.Join(os.Getenv("HOME"), ".zcompdump*")

		matches, _ := filepath.Glob(cachePattern)
		for _, match := range matches {
			_ = os.Remove(match)
		}

		// Add to fpath if using standard Zsh (not Oh-My-Zsh)
		if !dirExists(filepath.Join(os.Getenv("HOME"), ".oh-my-zsh")) {
			zshrc := filepath.Join(os.Getenv("HOME"), ".zshrc")
			if err := ensureFpathInZshrc(zshrc, compDir); err != nil {
				fmt.Printf("%s %s\n", warnStyle.Render("Warning:"), err)
			}
		}

		return nil
	}

	return installPath, installFunc
}

func installBash(rootCmd *cobra.Command, force bool) (string, func(*cobra.Command) error) {
	compDir := filepath.Join(os.Getenv("HOME"), ".bash_completions")
	installPath := filepath.Join(compDir, version.CliName)

	installFunc := func(rootCmd *cobra.Command) error {
		// Create directory
		if err := os.MkdirAll(compDir, 0755); err != nil {
			return err
		}

		// Create completion file
		f, err := os.Create(installPath)
		if err != nil {
			return err
		}
		defer f.Close()

		// Generate completion
		if err := rootCmd.GenBashCompletion(f); err != nil {
			return err
		}

		// Add sourcing to bashrc if not already there
		bashrc := filepath.Join(os.Getenv("HOME"), ".bashrc")
		if err := ensureSourceInBashrc(bashrc, installPath); err != nil {
			fmt.Printf("%s %s\n", warnStyle.Render("Warning:"), err)
		}

		return nil
	}

	return installPath, installFunc
}

func installFish(rootCmd *cobra.Command, force bool) (string, func(*cobra.Command) error) {
	compDir := filepath.Join(os.Getenv("HOME"), ".config", "fish", "completions")
	installPath := filepath.Join(compDir, version.CliName+".fish")

	installFunc := func(rootCmd *cobra.Command) error {
		// Create directory
		if err := os.MkdirAll(compDir, 0755); err != nil {
			return err
		}

		// Create completion file
		f, err := os.Create(installPath)
		if err != nil {
			return err
		}
		defer f.Close()

		// Generate completion
		return rootCmd.GenFishCompletion(f, true)
	}

	return installPath, installFunc
}

func showActivationInstructions(shell Shell) {
	fmt.Println(successStyle.Render("To activate completions:"))
	fmt.Println()

	switch shell {
	case ShellZsh:
		fmt.Println("  1. Restart your shell:")
		fmt.Println(infoStyle.Render("     exec zsh"))
		fmt.Println()
		fmt.Println("  2. Or reload your configuration:")
		fmt.Println(infoStyle.Render("     source ~/.zshrc"))
	case ShellBash:
		fmt.Println("  1. Restart your shell or reload configuration:")
		fmt.Println(infoStyle.Render("     source ~/.bashrc"))
	case ShellFish:
		fmt.Println("  1. Completions are active immediately")
		fmt.Println("  2. Or restart fish:")
		fmt.Println(infoStyle.Render("     exec fish"))
	}

	fmt.Println()
	fmt.Println("Test completions with:")
	fmt.Println(infoStyle.Render("  " + version.CliName + " <TAB>"))
	fmt.Println(infoStyle.Render("  " + version.CliName + " run <TAB>"))
}

func ensureFpathInZshrc(zshrc, compDir string) error {
	// Check if file exists
	if !fileExists(zshrc) {
		return fmt.Errorf("~/.zshrc not found, please create it first")
	}

	// Read file
	content, err := os.ReadFile(zshrc)
	if err != nil {
		return err
	}

	// Check if already contains the path
	if strings.Contains(string(content), compDir) {
		return nil
	}

	// Append to file
	f, err := os.OpenFile(zshrc, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(fmt.Sprintf("\n# Added by %s completion installer\n", version.CliName))
	if err != nil {
		return err
	}

	_, err = f.WriteString(fmt.Sprintf("fpath=(%s $fpath)\n", compDir))
	if err != nil {
		return err
	}

	_, err = f.WriteString("autoload -U compinit && compinit\n")

	return err
}

func ensureSourceInBashrc(bashrc, completionFile string) error {
	// Check if file exists
	if !fileExists(bashrc) {
		return fmt.Errorf("~/.bashrc not found, please create it first")
	}

	// Read file
	content, err := os.ReadFile(bashrc)
	if err != nil {
		return err
	}

	// Check if already contains the source line
	if strings.Contains(string(content), completionFile) {
		return nil
	}

	// Append to file
	f, err := os.OpenFile(bashrc, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(fmt.Sprintf("\n# Added by %s completion installer\n", version.CliName))
	if err != nil {
		return err
	}

	_, err = f.WriteString(fmt.Sprintf("[ -f %s ] && source %s\n", completionFile, completionFile))

	return err
}

func uninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall shell completions",
		Long:  `Uninstall shell completions by detecting your shell and removing from the standard location.`,
		Example: `  # Uninstall completions for your current shell
  ` + version.CliName + ` completion uninstall`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runUninstall()
		},
	}

	return cmd
}

func runUninstall() error {
	shell, err := detectShell()
	if err != nil {
		return err
	}

	fmt.Printf("%s Detected shell: %s\n", infoStyle.Render("→"), shell)
	fmt.Println()

	var removed bool

	switch Shell(shell) {
	case ShellZsh:
		removed = uninstallZsh()
	case ShellBash:
		removed = uninstallBash()
	case ShellFish:
		removed = uninstallFish()
	default:
		return fmt.Errorf("unsupported shell: %s", shell)
	}

	if removed {
		fmt.Printf("%s Completion uninstalled\n", successStyle.Render("✓"))
		fmt.Println()
		fmt.Println("Restart your shell to apply changes")
	} else {
		fmt.Printf("%s No completion found\n", infoStyle.Render("ℹ"))
	}

	return nil
}

func uninstallZsh() bool {
	var removed bool

	// Oh-My-Zsh location
	path1 := filepath.Join(os.Getenv("HOME"), ".oh-my-zsh", "custom", "completions", "_"+version.CliName)
	if fileExists(path1) {
		_ = os.Remove(path1)
		fmt.Printf("%s Removed: %s\n", successStyle.Render("✓"), path1)
		removed = true
	}

	// Standard Zsh location
	path2 := filepath.Join(os.Getenv("HOME"), ".zsh", "completions", "_"+version.CliName)
	if fileExists(path2) {
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
	if fileExists(path) {
		_ = os.Remove(path)
		fmt.Printf("%s Removed: %s\n", successStyle.Render("✓"), path)

		return true
	}

	return false
}

func uninstallFish() bool {
	path := filepath.Join(os.Getenv("HOME"), ".config", "fish", "completions", version.CliName+".fish")
	if fileExists(path) {
		_ = os.Remove(path)
		fmt.Printf("%s Removed: %s\n", successStyle.Render("✓"), path)

		return true
	}

	return false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}

	return !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}

	return info.IsDir()
}

func getShellExecutable() string {
	shell := os.Getenv("SHELL")
	if shell != "" {
		return shell
	}

	// Try to find in PATH
	for _, sh := range []string{"zsh", "bash", "fish"} {
		if path, err := exec.LookPath(sh); err == nil {
			return path
		}
	}

	return ""
}
