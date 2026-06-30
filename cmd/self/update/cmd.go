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
	"runtime"
	"strings"

	"github.com/datarobot/cli/internal/fsutil"
	"github.com/datarobot/cli/internal/log"
	internalShell "github.com/datarobot/cli/internal/shell"
	"github.com/datarobot/cli/internal/tools"
	"github.com/datarobot/cli/internal/version"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command { //nolint:cyclop
	var force bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "🔄 Update DataRobot CLI",
		Long: `Updates the DataRobot CLI to latest version. This will use Homebrew
to update if it detects the installed cask;  otherwise it will use an OS-appropriate script
with your default shell.
`,

		RunE: func(_ *cobra.Command, _ []string) error {
			requirement, err := tools.GetSelfRequirement()
			if err != nil {
				return err
			}

			if tools.SufficientSelfVersion(requirement.MinimumVersion) && !force {
				if requirement.MinimumVersion != "" {
					fmt.Fprintf(os.Stderr, "Required version: %s. ", requirement.MinimumVersion)
				}

				fmt.Fprintf(os.Stderr, "Installed version: %s.\n", version.Version)
				fmt.Fprintln(os.Stderr, "Skipping update. To force update to latest version, add -f flag.")

				return nil
			}

			// Account for when dr-cli cask has been installed - via `brew install datarobot-oss/taps/dr-cli`
			if runtime.GOOS == "darwin" { //nolint:nestif
				// Try to find brew and check if datarobot-oss is installed
				brewPath, err := exec.LookPath("brew")
				if err == nil {
					brewCheckCmd := exec.Command(brewPath, "list", "--cask", "dr-cli")

					// If we have dr-cli cask installed then attempt upgrade (err above indicates dr-cli wasn't found)
					if err := brewCheckCmd.Run(); err == nil {
						// Update brew first
						brewUpdateCmd := exec.Command(brewPath, "update")
						brewUpdateCmd.Stdout = os.Stdout
						brewUpdateCmd.Stderr = os.Stderr

						if err := brewUpdateCmd.Run(); err != nil {
							fmt.Fprintln(os.Stderr, "Error: ", err)
							return err
						}

						brewReinstallCmd := exec.Command(brewPath, "reinstall", "--cask", "dr-cli", "--force")
						brewReinstallCmd.Stdout = os.Stdout
						brewReinstallCmd.Stderr = os.Stderr

						if err := brewReinstallCmd.Run(); err != nil {
							fmt.Fprintln(os.Stderr, "Error: ", err)
							return err
						}

						return nil
					}
				}
			}

			// Now, assuming we haven't upgraded via brew handle with OS specific command
			shell, err := internalShell.DetectShell()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error while determining shell: ", err)
				return err
			}

			var (
				command    string
				executable string
				backup     string
			)

			switch runtime.GOOS {
			case "windows":
				command = "irm https://raw.githubusercontent.com/datarobot-oss/cli/main/install.ps1 | iex"

				var err error

				executable, backup, err = backupExecutable()
				if err != nil {
					return err
				}
			case "darwin", "linux":
				command = "curl -fsSL https://raw.githubusercontent.com/datarobot-oss/cli/main/install.sh | sh"
			default:
				return fmt.Errorf("could not determine OS: %s", runtime.GOOS)
			}

			execCmd := exec.Command(shell, "-c", command)

			execCmd.Stdout = os.Stdout
			execCmd.Stderr = os.Stderr

			// On Linux/macOS the install script (install.sh, see
			// https://raw.githubusercontent.com/datarobot-oss/cli/main/install.sh)
			// defaults INSTALL_DIR to
			// ~/.local/bin, ignoring where dr is actually installed. Point it at
			// the running binary's directory so the update lands in place instead
			// of scattering files (e.g. DataRobot Codespaces install dr under a
			// dr/ directory on PATH). Respect a user-provided INSTALL_DIR.
			if runtime.GOOS != "windows" {
				if _, ok := os.LookupEnv("INSTALL_DIR"); !ok {
					if dir, ok := resolveInstallDir(); ok {
						execCmd.Env = append(os.Environ(), "INSTALL_DIR="+dir)
					}
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

	return cmd
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
