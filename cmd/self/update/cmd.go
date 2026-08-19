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

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/datarobot/cli/internal/fsutil"
	"github.com/datarobot/cli/internal/log"
	internalShell "github.com/datarobot/cli/internal/shell"
	"github.com/datarobot/cli/internal/tools"
	"github.com/datarobot/cli/internal/version"
	"github.com/spf13/cobra"
)

// versionPattern requires a strict MAJOR.MINOR.PATCH numeric triplet, with an
// optional leading "v" and optional -prerelease / +build suffixes.
var versionPattern = regexp.MustCompile(`^v?\d+\.\d+\.\d+(-[0-9A-Za-z-.]+)?(\+[0-9A-Za-z-.]+)?$`)

func Cmd() *cobra.Command { //nolint:cyclop
	var (
		force         bool
		targetVersion string
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "🔄 Update DataRobot CLI",
		Long: `Updates the DataRobot CLI to latest version. This will use Homebrew
to update if it detects the installed cask;  otherwise it will use an OS-appropriate script
with your default shell.
`,

		RunE: func(_ *cobra.Command, _ []string) error {
			if targetVersion != "" {
				normalized, err := normalizeAndValidateVersion(targetVersion)
				if err != nil {
					return err
				}

				targetVersion = normalized

				if err := refuseDowngrade(targetVersion); err != nil {
					return err
				}
			}

			requirement, err := tools.GetSelfRequirement()
			if err != nil {
				return fmt.Errorf("get self requirement: %w", err)
			}

			if tools.SufficientSelfVersion(requirement.MinimumVersion) && !force && targetVersion == "" {
				if requirement.MinimumVersion != "" {
					fmt.Fprintf(os.Stderr, "Required version: %s. ", requirement.MinimumVersion)
				}

				fmt.Fprintf(os.Stderr, "Installed version: %s.\n", version.Version)
				fmt.Fprintln(os.Stderr, "Skipping update. To force update to latest version, add -f flag.")

				return nil
			}

			var (
				command    string
				executable string
				backup     string
				execCmd    *exec.Cmd
			)

			switch runtime.GOOS {
			case "windows":
				command = "irm https://cli.datarobot.com/winstall | iex"
				if targetVersion != "" {
					command = fmt.Sprintf("$env:VERSION='%s'; %s", targetVersion, command)
				}

				var err error

				executable, backup, err = backupExecutable()
				if err != nil {
					return err
				}

				// The install command is a PowerShell one-liner, so it must run
				// under PowerShell regardless of the detected shell (e.g. cmd.exe,
				// which uses /C and cannot run `irm ... | iex`).
				execCmd = exec.Command("powershell", "-NoProfile", "-Command", command)
			case "darwin", "linux":
				if handled, err := tryBrewUpdate(targetVersion); handled {
					return err
				}

				// Now, assuming we haven't upgraded via brew handle with OS specific command
				shell, err := internalShell.DetectShell()
				if err != nil {
					fmt.Fprintln(os.Stderr, "Error while determining shell: ", err)
					return fmt.Errorf("detect shell: %w", err)
				}

				command = "curl -fsSL https://cli.datarobot.com/install | sh"
				if targetVersion != "" {
					command += " -s -- " + targetVersion
				}

				execCmd = exec.Command(shell, "-c", command)
			default:
				return fmt.Errorf("could not determine OS: %s", runtime.GOOS)
			}

			execCmd.Stdout = os.Stdout
			execCmd.Stderr = os.Stderr

			// The install scripts (install.sh, install.ps1) default INSTALL_DIR to
			// ~/.local/bin or %LOCALAPPDATA%\Programs\dr, ignoring where dr is
			// actually installed. Point it at the running binary's directory so the
			// update lands in place (e.g. DataRobot Codespaces install dr under a
			// dr/ directory on PATH). Respect a user-provided INSTALL_DIR.
			if _, ok := os.LookupEnv("INSTALL_DIR"); !ok {
				if dir, ok := resolveInstallDir(); ok {
					execCmd.Env = append(os.Environ(), "INSTALL_DIR="+dir)
				}
			}

			if err := execCmd.Run(); err != nil {
				if runtime.GOOS == "windows" {
					// rename back if update failed
					revertErr := os.Rename(backup, executable)
					if revertErr != nil {
						log.Errorf("Could not revert executable from backup: %s\n", backup)
					}
				}

				return fmt.Errorf("command execution failed: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force update to latest version")
	cmd.Flags().StringVar(&targetVersion, "version", "", "Install a specific released version instead of latest (e.g. v0.12.3)")

	return cmd
}

// normalizeAndValidateVersion validates s as a strict semantic version
// (vMAJOR.MINOR.PATCH, optionally without the leading "v", optionally with a
// -prerelease and/or +build suffix) and returns it normalized to "vX.Y.Z...".
func normalizeAndValidateVersion(s string) (string, error) {
	trimmed := strings.TrimSpace(s)
	if !versionPattern.MatchString(trimmed) {
		return "", fmt.Errorf("invalid version %q: expected vMAJOR.MINOR.PATCH (e.g. v0.12.3)", s)
	}

	if strings.HasPrefix(trimmed, "v") {
		return trimmed, nil
	}

	return "v" + trimmed, nil
}

// refuseDowngrade errors if requestedVersion is older than the currently
// running dr version (version.Version). Skipped entirely for non-release
// ("dev") builds.
func refuseDowngrade(requestedVersion string) error {
	if version.Version == "dev" {
		return nil
	}

	installed, err := semver.NewVersion(version.Version)
	if err != nil {
		return nil
	}

	requested, err := semver.NewVersion(requestedVersion)
	if err != nil {
		return fmt.Errorf("invalid version %q: expected vMAJOR.MINOR.PATCH (e.g. v0.12.3)", requestedVersion)
	}

	if requested.LessThan(installed) {
		return fmt.Errorf("refusing to install older version (installed: %s, requested: %s)", version.Version, requestedVersion)
	}

	return nil
}

// isBrewCaskInstalled reports whether the dr-cli Homebrew cask is installed,
// given a resolved brew path.
func isBrewCaskInstalled(brewPath string) bool {
	return exec.Command(brewPath, "list", "--cask", "dr-cli").Run() == nil
}

// brewCaskVersionError is returned when --version is requested but dr was
// installed via the Homebrew cask, which cannot pin specific versions.
func brewCaskVersionError(targetVersion string) error {
	return fmt.Errorf(
		"dr was installed via Homebrew (dr-cli cask). Homebrew always installs the latest release and cannot pin versions.\n\n"+
			"To install %s manually, uninstall the cask and install with the installation script:\n\n"+
			"  brew uninstall --cask dr-cli\n"+
			"  curl -fsSL https://cli.datarobot.com/install | sh -s -- %s\n",
		targetVersion, targetVersion)
}

// tryBrewUpdate checks whether dr was installed via the Homebrew cask and,
// if so, either hard-errors (targetVersion set — Homebrew can't pin
// versions) or updates via brew directly. handled is true whenever the
// brew-managed path was taken (regardless of success), signaling the caller
// should return err immediately rather than falling through to the generic
// install-script path.
func tryBrewUpdate(targetVersion string) (handled bool, err error) {
	brewPath, err := exec.LookPath("brew")
	if err != nil {
		return false, nil
	}

	if !isBrewCaskInstalled(brewPath) {
		return false, nil
	}

	if targetVersion != "" {
		return true, brewCaskVersionError(targetVersion)
	}

	brewUpdateCmd := exec.Command(brewPath, "update")
	brewUpdateCmd.Stdout = os.Stdout
	brewUpdateCmd.Stderr = os.Stderr

	if err := brewUpdateCmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error: ", err)
		return true, fmt.Errorf("brew update: %w", err)
	}

	brewReinstallCmd := exec.Command(brewPath, "reinstall", "--cask", "dr-cli", "--force")
	brewReinstallCmd.Stdout = os.Stdout
	brewReinstallCmd.Stderr = os.Stderr

	if err := brewReinstallCmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error: ", err)
		return true, fmt.Errorf("brew reinstall: %w", err)
	}

	return true, nil
}

// resolveInstallDir returns the directory of the currently running executable,
// resolving any symlinks first so the path points at the real binary. It is used
// to tell the install script where dr actually lives. The boolean is false when
// the executable path cannot be determined, in which case callers should fall
// back to the install script's default location.
func resolveInstallDir() (string, bool) {
	executable, err := os.Executable()
	if err != nil {
		return "", false
	}

	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}

	return filepath.Dir(executable), true
}

// backupExecutable creates a backup of the current CLI executable before updating.
// It renames the existing executable to a versioned backup file (e.g., dr_v1.2.3).
// If a backup from the same version already exists, it is removed first to avoid conflicts.
//
// Returns:
//   - executable: absolute path to the original CLI executable
//   - backup: absolute path to the backup file (with version suffix)
//   - error: if determining the executable path, removing old backups, or creating the backup fails
func backupExecutable() (string, string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", "", fmt.Errorf("could not determine current executable: %w", err)
	}

	dir, file := filepath.Split(executable)
	ext := filepath.Ext(file)
	name := strings.TrimSuffix(file, ext)

	backup := filepath.Join(dir, name+"_"+version.Version+ext)

	if fsutil.FileExists(backup) {
		err = os.Remove(backup)
		if err != nil {
			return "", "", fmt.Errorf("could not remove old backup executable %s: %w", backup, err)
		}
	}

	err = os.Rename(executable, backup)
	if err != nil {
		return "", "", fmt.Errorf("could not backup current executable: %w", err)
	}

	return executable, backup, nil
}
