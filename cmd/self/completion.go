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
	"os"
	"strings"

	internalShell "github.com/datarobot/cli/internal/shell"
	"github.com/datarobot/cli/internal/version"
	"github.com/spf13/cobra"
)

func supportedShells() []string {
	return []string{
		string(internalShell.Bash),
		string(internalShell.Zsh),
		string(internalShell.Fish),
		string(internalShell.PowerShell),
	}
}

func CompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("completion [%s]", strings.Join(supportedShells(), "|")),
		Short: "Generate or manage shell completion scripts.",
		Long: `Generate shell completion script for supported shells. This will be output
		to stdout so it can be redirected to the appropriate location.

You can also use the 'install' subcommand to install completions interactively.`,
		Example: `To load completions:

Bash:

  $ source <(` + version.CliName + ` completion bash)

  # To load completions for each session, execute once:

  # Linux:
  $ ` + version.CliName + ` completion bash > /etc/bash_completion.d/` + version.CliName + `

Zsh:

  # If shell completion is not already enabled in your environment you will need
  # to enable it. You can execute the following to do so:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # Linux or MacOS:
  $ ` + version.CliName + ` completion zsh > ${ZDOTDIR:-$HOME}/.zsh/completions/_dr` + version.CliName + `

Fish:

  $ ` + version.CliName + ` completion fish | source

  # To load completions for each session, execute once:
  $ ` + version.CliName + ` completion fish > ~/.config/fish/completions/` + version.CliName + `.fish

PowerShell:

  PS> ` + version.CliName + ` completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> ` + version.CliName + ` completion powershell > ` + version.CliName + `.ps1
  # and source it from your PowerShell profile.
`,
		DisableFlagsInUseLine: true,
		Args:                  cobra.MatchAll(cobra.ExactArgs(1)),
		ValidArgs:             supportedShells(),
		RunE: func(cmd *cobra.Command, args []string) error {
			shell := internalShell.Shell(args[0])

			switch shell {
			case internalShell.Bash:
				return cmd.Root().GenBashCompletion(os.Stdout)
			case internalShell.Zsh:
				// Cobra v1.1.1+ supports GenZshCompletion
				return cmd.Root().GenZshCompletion(os.Stdout)
			case internalShell.Fish:
				// the `true` gives fish the “__fish_use_subcommand” behavior
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			case internalShell.PowerShell:
				return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				return fmt.Errorf("Unsupported shell %q.", args[0])
			}
		},
	}

	// Add subcommands
	cmd.AddCommand(
		installCompletionCmd(),
		uninstallCompletionCmd(),
	)

	return cmd
}
