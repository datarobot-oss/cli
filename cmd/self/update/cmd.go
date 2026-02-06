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

package update

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

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
		Short: "Update DataRobot CLI",
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

						brewUpgradeCmd := exec.Command(brewPath, "upgrade", "--cask", "dr-cli")
						brewUpgradeCmd.Stdout = os.Stdout
						brewUpgradeCmd.Stderr = os.Stderr

						if err := brewUpgradeCmd.Run(); err != nil {
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

			var command string

			switch runtime.GOOS {
			case "windows":
				command = "irm https://raw.githubusercontent.com/datarobot-oss/cli/main/install.ps1 | iex"
			case "darwin", "linux":
				command = "curl -fsSL https://raw.githubusercontent.com/datarobot-oss/cli/main/install.sh | sh"
			default:
				log.Fatalf("Could not determine OS: %s\n", runtime.GOOS)
			}

			execCmd := exec.Command(shell, "-c", command)

			execCmd.Stdout = os.Stdout
			execCmd.Stderr = os.Stderr

			if err := execCmd.Run(); err != nil {
				log.Fatalf("Command execution failed: %v", err)
				return err
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force update to latest version")

	return cmd
}
