// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package self

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"

	internalShell "github.com/datarobot/cli/internal/shell"
	"github.com/spf13/cobra"
)

func UpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update DataRobot CLI",
		Long: `Updates the DataRobot CLI to latest version. This will use Homebrew
to update if it detects the installed cask;  otherwise it will use an OS-appropriate script
with your default shell.
`,
		Run: func(_ *cobra.Command, _ []string) {
			// Account for when dr-cli cask has been installed - via `brew install datarobot-oss/taps/dr-cli`
			if runtime.GOOS == "darwin" { //nolint:nestif
				// Try to find brew and check if datarobot-oss is installed
				brewPath, err := exec.LookPath("brew")
				if err == nil {
					brewCheckCmd := exec.Command(brewPath, "list", "--cask", "dr-cli")

					// If we have dr-cli cask installed then attempt upgrade (err indicates it wasn't found)
					if err := brewCheckCmd.Run(); err == nil {
						brewUpdateCmd := exec.Command(brewPath, "update", "datarobot-oss/tap")
						brewUpdateCmd.Stdout = os.Stdout
						brewUpdateCmd.Stderr = os.Stderr

						if err := brewUpdateCmd.Run(); err != nil {
							fmt.Fprintln(os.Stderr, "Error:", err)
							os.Exit(1)
						}

						brewUpgradeCmd := exec.Command(brewPath, "upgrade", "--cask", "dr-cli")
						brewUpgradeCmd.Stdout = os.Stdout
						brewUpgradeCmd.Stderr = os.Stderr

						if err := brewUpgradeCmd.Run(); err != nil {
							fmt.Fprintln(os.Stderr, "Error:", err)
							os.Exit(1)
						}

						return
					}
				}
			}

			// Now, assuming we haven't upgraded via brew handle with OS specific command
			shell, err := internalShell.DetectShell()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error while determining shell:", err)
				os.Exit(1)

				return
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
			}
		},
	}

	return cmd
}
