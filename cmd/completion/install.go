// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package completion

import (
	"errors"
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
	infoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
)

func installCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "install [shell]",
		Short: "Install shell completions interactively",
		Long: `Install shell completions automatically by detecting your shell and
installing to the appropriate location.

This command will:
- Detect your current shell (or use specified shell)
- Install completions to the standard location
- Clear completion cache (if needed)
- Show instructions to activate completions`,
		Example: `  # Install completions for your current shell
  ` + version.CliName + ` completion install

  # Install completions for a specific shell
  ` + version.CliName + ` completion install bash
  ` + version.CliName + ` completion install zsh

  # Force reinstall even if already installed
  ` + version.CliName + ` completion install --force`,
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: supportedShells(),
		RunE: func(cmd *cobra.Command, args []string) error {
			var shell string
			if len(args) > 0 {
				shell = args[0]
			}

			return runInstall(cmd.Root(), shell, force)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force reinstall even if already installed")

	return cmd
}

func runInstall(rootCmd *cobra.Command, specifiedShell string, force bool) error {
	shell, err := resolveShell(specifiedShell)
	if err != nil {
		return err
	}

	fmt.Println()

	shellType := Shell(shell)

	installPath, installFunc, err := getInstallFunc(rootCmd, shellType, force)
	if err != nil {
		return err
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
	showActivationInstructions(shellType)

	return nil
}

func resolveShell(specifiedShell string) (string, error) {
	if specifiedShell != "" {
		// Use specified shell
		fmt.Printf("%s Installing for shell: %s\n", infoStyle.Render("→"), specifiedShell)

		return specifiedShell, nil
	}

	// Detect current shell
	shell, err := detectShell()
	if err != nil {
		return "", err
	}

	fmt.Printf("%s Detected shell: %s\n", infoStyle.Render("→"), shell)

	return shell, nil
}

func getInstallFunc(rootCmd *cobra.Command, shellType Shell, force bool) (string, func(*cobra.Command) error, error) {
	switch shellType {
	case ShellZsh:
		path, fn := installZsh(rootCmd, force)
		return path, fn, nil
	case ShellBash:
		path, fn := installBash(rootCmd, force)
		return path, fn, nil
	case ShellFish:
		path, fn := installFish(rootCmd, force)
		return path, fn, nil
	case ShellPowerShell:
		return "", nil, errors.New("PowerShell completions installation not yet supported via this command. Use: dr completion powershell")
	default:
		return "", nil, fmt.Errorf("unsupported shell: %s", shellType)
	}
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

	return "", errors.New("could not detect shell. Please set SHELL environment variable")
}

func installZsh(_ *cobra.Command, _ bool) (string, func(*cobra.Command) error) {
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
		if err := os.MkdirAll(compDir, 0o755); err != nil {
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

func installBash(_ *cobra.Command, _ bool) (string, func(*cobra.Command) error) {
	compDir := filepath.Join(os.Getenv("HOME"), ".bash_completions")
	installPath := filepath.Join(compDir, version.CliName)

	installFunc := func(rootCmd *cobra.Command) error {
		// Check if bash-completion is available
		if !isBashCompletionAvailable() {
			fmt.Println()
			fmt.Printf("%s Bash completion framework not detected\n", warnStyle.Render("⚠"))
			fmt.Println()
			fmt.Println("Bash completions require the bash-completion package.")
			fmt.Println()
			fmt.Println("To install:")
			fmt.Println()

			if runtime.GOOS == "darwin" {
				fmt.Println(infoStyle.Render("  # macOS (Homebrew)"))
				fmt.Println(infoStyle.Render("  brew install bash-completion@2"))
				fmt.Println()
				fmt.Println(infoStyle.Render("  # Then add to ~/.bash_profile:"))
				fmt.Println(infoStyle.Render(`  export BASH_COMPLETION_COMPAT_DIR="/opt/homebrew/etc/bash_completion.d"`))
				fmt.Println(infoStyle.Render(`  [[ -r "/opt/homebrew/etc/profile.d/bash_completion.sh" ]] && . "/opt/homebrew/etc/profile.d/bash_completion.sh"`))
			} else {
				fmt.Println(infoStyle.Render("  # Ubuntu/Debian"))
				fmt.Println(infoStyle.Render("  sudo apt-get install bash-completion"))
				fmt.Println()
				fmt.Println(infoStyle.Render("  # RHEL/CentOS"))
				fmt.Println(infoStyle.Render("  sudo yum install bash-completion"))
			}

			fmt.Println()
			fmt.Println("After installing bash-completion, run this command again.")
			fmt.Println()

			return errors.New("bash-completion not available")
		}

		// Create directory
		if err := os.MkdirAll(compDir, 0o755); err != nil {
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

func isBashCompletionAvailable() bool {
	// Check if bash-completion is installed by looking for the main completion file
	// or checking if the _get_comp_words_by_ref function would be available

	// Common locations for bash-completion
	locations := []string{
		"/usr/share/bash-completion/bash_completion",
		"/etc/bash_completion",
		"/usr/local/etc/bash_completion",                 // Homebrew on older systems
		"/opt/homebrew/etc/profile.d/bash_completion.sh", // Homebrew on Apple Silicon
		"/usr/local/etc/profile.d/bash_completion.sh",    // Homebrew on Intel Macs
	}

	for _, loc := range locations {
		if fileExists(loc) {
			return true
		}
	}

	// Also check if bash-completion is in Homebrew
	if runtime.GOOS == "darwin" {
		// Try to find brew and check if bash-completion is installed
		brewPath, err := exec.LookPath("brew")
		if err == nil {
			cmd := exec.Command(brewPath, "list", "bash-completion@2")
			if err := cmd.Run(); err == nil {
				return true
			}
		}
	}

	return false
}

func installFish(_ *cobra.Command, _ bool) (string, func(*cobra.Command) error) {
	compDir := filepath.Join(os.Getenv("HOME"), ".config", "fish", "completions")
	installPath := filepath.Join(compDir, version.CliName+".fish")

	installFunc := func(rootCmd *cobra.Command) error {
		// Create directory
		if err := os.MkdirAll(compDir, 0o755); err != nil {
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
		fmt.Println("  1. Make sure bash-completion is installed (see above if needed)")
		fmt.Println()
		fmt.Println("  2. Restart your shell or reload configuration:")
		fmt.Println(infoStyle.Render("     source ~/.bashrc"))
		fmt.Println()
		fmt.Println("  3. If using macOS, make sure your ~/.bash_profile sources ~/.bashrc:")
		fmt.Println(infoStyle.Render(`     echo "[ -r ~/.bashrc ] && . ~/.bashrc" >> ~/.bash_profile`))
	case ShellFish:
		fmt.Println("  1. Completions are active immediately")
		fmt.Println("  2. Or restart fish:")
		fmt.Println(infoStyle.Render("     exec fish"))
	case ShellPowerShell:
		fmt.Println("  1. Source the completion script in your PowerShell profile")
		fmt.Println(infoStyle.Render("     See: dr completion powershell --help"))
	}

	fmt.Println()
	fmt.Println("Test completions with:")
	fmt.Println(infoStyle.Render("  " + version.CliName + " <TAB>"))
	fmt.Println(infoStyle.Render("  " + version.CliName + " run <TAB>"))
}

func ensureFpathInZshrc(zshrc, compDir string) error {
	// Check if file exists
	if !fileExists(zshrc) {
		return errors.New("~/.zshrc not found, please create it first")
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
	f, err := os.OpenFile(zshrc, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err = fmt.Fprintf(f, "\n# Added by %s completion installer\n", version.CliName); err != nil {
		return err
	}

	if _, err = fmt.Fprintf(f, "fpath=(%s $fpath)\n", compDir); err != nil {
		return err
	}

	_, err = f.WriteString("autoload -U compinit && compinit\n")

	return err
}

func ensureSourceInBashrc(bashrc, completionFile string) error {
	// Check if file exists
	if !fileExists(bashrc) {
		return errors.New("~/.bashrc not found, please create it first")
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
	f, err := os.OpenFile(bashrc, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err = fmt.Fprintf(f, "\n# Added by %s completion installer\n", version.CliName); err != nil {
		return err
	}

	_, err = fmt.Fprintf(f, "[ -f %s ] && source %s\n", completionFile, completionFile)

	return err
}

func uninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall [shell]",
		Short: "Uninstall shell completions",
		Long:  `Uninstall shell completions by detecting your shell and removing from the standard location.`,
		Example: `  # Uninstall completions for your current shell
  ` + version.CliName + ` completion uninstall

  # Uninstall completions for a specific shell
  ` + version.CliName + ` completion uninstall bash
  ` + version.CliName + ` completion uninstall zsh`,
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: supportedShells(),
		RunE: func(_ *cobra.Command, args []string) error {
			var shell string
			if len(args) > 0 {
				shell = args[0]
			}

			return runUninstall(shell)
		},
	}

	return cmd
}

func runUninstall(specifiedShell string) error {
	var shell string

	var err error

	if specifiedShell != "" {
		// Use specified shell
		shell = specifiedShell
		fmt.Printf("%s Uninstalling for shell: %s\n", infoStyle.Render("→"), shell)
	} else {
		// Detect current shell
		shell, err = detectShell()
		if err != nil {
			return err
		}

		fmt.Printf("%s Detected shell: %s\n", infoStyle.Render("→"), shell)
	}

	fmt.Println()

	var removed bool

	switch Shell(shell) {
	case ShellZsh:
		removed = uninstallZsh()
	case ShellBash:
		removed = uninstallBash()
	case ShellFish:
		removed = uninstallFish()
	case ShellPowerShell:
		return errors.New("PowerShell completions uninstallation not yet supported via this command")
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
