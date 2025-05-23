// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/datarobot/cli/internal/version"
	"github.com/spf13/cobra"
)

type Shell string

const (
	ShellBash       Shell = "bash"
	ShellZsh        Shell = "zsh"
	ShellFish       Shell = "fish"
	ShellPowerShell Shell = "powershell"
)

func supportedShells() []string {
	return []string{
		string(ShellBash),
		string(ShellZsh),
		string(ShellFish),
		string(ShellPowerShell),
	}
}

func completionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("completion [%s]", strings.Join(supportedShells(), "|")),
		Short: "Generate shell completion script",
		Long: `To load completions:

Bash:

  $ source <(` + version.AppName + ` completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ ` + version.AppName + ` completion bash > /etc/bash_completion.d/` + version.AppName + `

  # macOS (with Homebrew):
  $ ` + version.AppName + ` completion bash > /usr/local/etc/bash_completion.d/` + version.AppName + `

Zsh:

  # If shell completion is not already enabled in your environment you will need
  # to enable it.  You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ ` + version.AppName + ` completion zsh > "${fpath[1]}/_` + version.AppName + `"

Fish:

  $ ` + version.AppName + ` completion fish | source

  # To load completions for each session, execute once:
  $ ` + version.AppName + ` completion fish > ~/.config/fish/completions/` + version.AppName + `.fish

PowerShell:

  PS> ` + version.AppName + ` completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> ` + version.AppName + ` completion powershell > ` + version.AppName + `.ps1
  # and source it from your PowerShell profile.
`,
		DisableFlagsInUseLine: true,
		Args:                  cobra.MatchAll(cobra.ExactArgs(1)),
		ValidArgs:             supportedShells(),
		RunE: func(_ *cobra.Command, args []string) error {
			shell := Shell(args[0])

			switch shell {
			case ShellBash:
				return RootCmd.GenBashCompletion(os.Stdout)
			case ShellZsh:
				// Cobra v1.1.1+ supports GenZshCompletion
				return RootCmd.GenZshCompletion(os.Stdout)
			case ShellFish:
				// the `true` gives fish the “__fish_use_subcommand” behavior
				return RootCmd.GenFishCompletion(os.Stdout, true)
			case ShellPowerShell:
				return RootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				return fmt.Errorf("unsupported shell %q", args[0])
			}
		},
	}

	return cmd
}
