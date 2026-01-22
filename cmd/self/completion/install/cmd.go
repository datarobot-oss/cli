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

package install

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	var force bool

	var yes bool

	var dryRun bool

	cmd := &cobra.Command{
		Use:   "install [shell]",
		Short: "Install shell completions interactively.",
		Long: `Install shell completions automatically by detecting your shell and
installing to the appropriate location.

This command will:
- Detect your current shell (or use specified shell)
- Show what will be installed and where
- Ask for confirmation (unless '--yes' is specified)
- Install completions to the standard location
- Clear completion cache (if needed)
- Show instructions to activate completions

By default, this command runs in preview mode. Use '--yes' to install directly.`,
		Example: `  # Preview what would be installed (default behavior):
  ` + version.CliName + ` completion install

  # Install completions for your current shell:
  ` + version.CliName + ` completion install --yes

  # Install completions for a specific shell:
  ` + version.CliName + ` completion install bash --yes
  ` + version.CliName + ` completion install zsh --yes

  # Preview installation for a specific shell:
  ` + version.CliName + ` completion install bash

  # Force reinstall, even if completions are already installed:
  ` + version.CliName + ` completion install --force --yes`,
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

			return runInstall(cmd.Root(), shell, force, yes, effectiveDryRun)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force reinstall, even if completions are already installed.")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Automatically confirm installation without prompting.")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview mode: show what would be installed without making changes.")

	return cmd
}

func runInstall(rootCmd *cobra.Command, specifiedShell string, force, yes, dryRun bool) error {
	shell, err := internalShell.ResolveShell(specifiedShell)
	if err != nil {
		return err
	}

	fmt.Println()

	shellType := internalShell.Shell(shell)

	installPath, installFunc, err := getInstallFunc(rootCmd, shellType, force)
	if err != nil {
		return err
	}

	// Check if already installed
	alreadyInstalled := fsutil.FileExists(installPath)
	if !force && alreadyInstalled {
		showAlreadyInstalled(installPath)
		return nil
	}

	// Show installation plan
	showInstallationPlan(shell, installPath, alreadyInstalled)

	// Dry-run mode
	if dryRun {
		showDryRunMessage(shell)
		return nil
	}

	// Ask for confirmation if not auto-confirmed
	if !yes {
		confirmed, err := promptForConfirmation()
		if err != nil {
			return err
		}

		if !confirmed {
			return nil
		}
	}

	fmt.Println()

	// Install
	if err := installFunc(rootCmd); err != nil {
		return fmt.Errorf("Failed to install completions: %w", err)
	}

	fmt.Printf("%s Completion installed to: %s.\n", successStyle.Render("✓"), installPath)
	fmt.Println()

	// Show activation instructions
	showActivationInstructions(shellType)

	return nil
}

func showAlreadyInstalled(installPath string) {
	fmt.Printf("%s Completion already installed at: %s.\n", successStyle.Render("✓"), installPath)
	fmt.Println()
	fmt.Println(infoStyle.Render("To reinstall, use: " + version.CliName + " completion install --force --yes"))
}

func showInstallationPlan(shell, installPath string, alreadyInstalled bool) {
	fmt.Println(infoStyle.Render("Installation Plan:"))
	fmt.Printf("  Shell:        %s\n", shell)
	fmt.Printf("  Install to:   %s\n", installPath)

	if alreadyInstalled {
		fmt.Printf("  Action:       %s (reinstall)\n", warnStyle.Render("Overwrite"))
	} else {
		fmt.Printf("  Action:       %s\n", successStyle.Render("Create new"))
	}

	fmt.Println()
}

func showDryRunMessage(shell string) {
	fmt.Println(infoStyle.Render("🔍 Dry-run mode (no changes will be made)"))
	fmt.Println()
	fmt.Println("To proceed with installation, run:")
	fmt.Println(infoStyle.Render("  " + version.CliName + " completion install " + shell + " --yes"))
}

func promptForConfirmation() (bool, error) {
	fmt.Print("Proceed with installation? [y/N]: ")

	response, err := reader.ReadString()
	if err != nil {
		return false, fmt.Errorf("Failed to read input: %w", err)
	}

	response = strings.ToLower(strings.TrimSpace(response))
	if response != "y" && response != "yes" {
		fmt.Println()
		fmt.Println(infoStyle.Render("Installation cancelled."))

		return false, nil
	}

	return true, nil
}

func getInstallFunc(rootCmd *cobra.Command, shellType internalShell.Shell, force bool) (string, func(*cobra.Command) error, error) {
	switch shellType {
	case internalShell.Zsh:
		path, fn := installZsh(rootCmd, force)
		return path, fn, nil
	case internalShell.Bash:
		path, fn := installBash(rootCmd, force)
		return path, fn, nil
	case internalShell.Fish:
		path, fn := installFish(rootCmd, force)
		return path, fn, nil
	case internalShell.PowerShell:
		path, fn := installPowerShell(rootCmd, force)
		return path, fn, nil
	default:
		return "", nil, fmt.Errorf("Unsupported shell: %s.", shellType)
	}
}

func installZsh(_ *cobra.Command, _ bool) (string, func(*cobra.Command) error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", func(*cobra.Command) error { return err }
	}

	var installPath string

	var compDir string

	// Check for Oh-My-Zsh first (most common)
	if fsutil.DirExists(filepath.Join(homeDir, ".oh-my-zsh")) {
		compDir = filepath.Join(homeDir, ".oh-my-zsh", "custom", "completions")
		installPath = filepath.Join(compDir, "_"+version.CliName)
	} else {
		// Use standard Zsh completion directory
		compDir = filepath.Join(homeDir, ".zsh", "completions")
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
		cachePattern := filepath.Join(homeDir, ".zcompdump*")

		matches, _ := filepath.Glob(cachePattern)
		for _, match := range matches {
			_ = os.Remove(match)
		}

		// Add to fpath if using standard Zsh (not Oh-My-Zsh)
		if !fsutil.DirExists(filepath.Join(homeDir, ".oh-my-zsh")) {
			zshrc := filepath.Join(homeDir, ".zshrc")
			if err := ensureFpathInZshrc(zshrc, compDir); err != nil {
				fmt.Printf("%s %s\n", warnStyle.Render("Warning: "), err)
			}
		}

		return nil
	}

	return installPath, installFunc
}

func installBash(_ *cobra.Command, _ bool) (string, func(*cobra.Command) error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", func(*cobra.Command) error { return err }
	}

	compDir := filepath.Join(homeDir, ".bash_completions")
	installPath := filepath.Join(compDir, version.CliName)

	installFunc := func(rootCmd *cobra.Command) error {
		// Check if bash-completion is available
		if !isBashCompletionAvailable() {
			fmt.Println()
			fmt.Printf("%s Bash completion framework not detected.\n", warnStyle.Render("⚠"))
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

			return errors.New("Bash-completion not available.")
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
		bashrc := filepath.Join(homeDir, ".bashrc")
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
		if fsutil.FileExists(loc) {
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
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", func(*cobra.Command) error { return err }
	}

	compDir := filepath.Join(homeDir, ".config", "fish", "completions")
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

func installPowerShell(_ *cobra.Command, _ bool) (string, func(*cobra.Command) error) {
	// Get PowerShell profile path
	var profilePath string

	if runtime.GOOS == "windows" {
		// On Windows, use $PROFILE (Documents\PowerShell or Documents\WindowsPowerShell)
		documentsPath, err := os.UserHomeDir()
		if err != nil {
			return "", func(*cobra.Command) error { return err }
		}

		documentsPath = filepath.Join(documentsPath, "Documents")

		// Try PowerShell Core first (PowerShell 7+)
		psCorePath := filepath.Join(documentsPath, "PowerShell")
		if fsutil.DirExists(psCorePath) {
			profilePath = filepath.Join(psCorePath, "Microsoft.PowerShell_profile.ps1")
		} else {
			// Fall back to Windows PowerShell 5.x
			profilePath = filepath.Join(documentsPath, "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1")
		}
	} else {
		// On Unix-like systems with PowerShell Core
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", func(*cobra.Command) error { return err }
		}

		profilePath = filepath.Join(homeDir, ".config", "powershell", "Microsoft.PowerShell_profile.ps1")
	}

	installFunc := func(rootCmd *cobra.Command) error {
		// Ensure the directory exists
		profileDir := filepath.Dir(profilePath)
		if err := os.MkdirAll(profileDir, 0o755); err != nil {
			return fmt.Errorf("Failed to create PowerShell profile directory: %w", err)
		}

		// Create or append to profile
		completionScript := fmt.Sprintf("\n# %s completion\n", version.CliName)
		completionScript += fmt.Sprintf("if (Get-Command %s -ErrorAction SilentlyContinue) {\n", version.CliName)
		completionScript += fmt.Sprintf("    %s completion powershell | Out-String | Invoke-Expression\n", version.CliName)
		completionScript += "}\n"

		// Check if profile exists and if completion is already set up
		if fsutil.FileExists(profilePath) {
			content, err := os.ReadFile(profilePath)
			if err != nil {
				return fmt.Errorf("Failed to read PowerShell profile: %w", err)
			}

			// Check if completion is already configured
			if strings.Contains(string(content), version.CliName+" completion powershell") {
				return nil // Already installed
			}
		}

		// Append to profile
		f, err := os.OpenFile(profilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("Failed to open PowerShell profile: %w", err)
		}
		defer f.Close()

		if _, err = f.WriteString(completionScript); err != nil {
			return fmt.Errorf("Failed to write to PowerShell profile: %w", err)
		}

		return nil
	}

	return profilePath, installFunc
}

func showActivationInstructions(shell internalShell.Shell) {
	fmt.Println(successStyle.Render("To activate completions:"))
	fmt.Println()

	switch shell {
	case internalShell.Zsh:
		fmt.Println("  1. Restart your shell:")
		fmt.Println(infoStyle.Render("     exec zsh"))
		fmt.Println()
		fmt.Println("  2. Or reload your configuration:")
		fmt.Println(infoStyle.Render("     source ~/.zshrc"))
	case internalShell.Bash:
		fmt.Println("  1. Make sure bash-completion is installed (see above if needed)")
		fmt.Println()
		fmt.Println("  2. Restart your shell or reload configuration:")
		fmt.Println(infoStyle.Render("     source ~/.bashrc"))
		fmt.Println()
		fmt.Println("  3. If using macOS, make sure your ~/.bash_profile sources ~/.bashrc:")
		fmt.Println(infoStyle.Render(`     echo "[ -r ~/.bashrc ] && . ~/.bashrc" >> ~/.bash_profile`))
	case internalShell.Fish:
		fmt.Println("  1. Completions are active immediately")
		fmt.Println("  2. Or restart fish:")
		fmt.Println(infoStyle.Render("     exec fish"))
	case internalShell.PowerShell:
		fmt.Println("  1. Restart PowerShell to load the updated profile:")
		fmt.Println(infoStyle.Render("     # Close and reopen PowerShell"))
		fmt.Println()
		fmt.Println("  2. Or reload your profile in the current session:")
		fmt.Println(infoStyle.Render("     . $PROFILE"))
	}

	fmt.Println()
	fmt.Println("Test completions with:")
	fmt.Println(infoStyle.Render("  " + version.CliName + " <TAB>"))
	fmt.Println(infoStyle.Render("  " + version.CliName + " run <TAB>"))
}

func ensureFpathInZshrc(zshrc, compDir string) error {
	// Check if file exists
	if !fsutil.FileExists(zshrc) {
		return errors.New("~/.zshrc not found, please create it first.")
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
	if !fsutil.FileExists(bashrc) {
		return errors.New("~/.bashrc not found, please create it first.")
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
