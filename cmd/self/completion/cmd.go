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

package completion

import (
	"fmt"
	"os"
	"strings"

	"github.com/datarobot/cli/cmd/self/completion/install"
	"github.com/datarobot/cli/cmd/self/completion/uninstall"
	internalShell "github.com/datarobot/cli/internal/shell"
	"github.com/datarobot/cli/internal/version"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("completion [%s]", strings.Join(internalShell.SupportedShells(), "|")),
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
		ValidArgs:             internalShell.SupportedShells(),
		RunE: func(cmd *cobra.Command, args []string) error {
			shell := internalShell.Shell(args[0])

			switch shell {
			case internalShell.Bash:
				return cmd.Root().GenBashCompletion(os.Stdout)
			case internalShell.Zsh:
				// Cobra v1.1.1+ supports GenZshCompletion
				return cmd.Root().GenZshCompletion(os.Stdout)
			case internalShell.Fish:
				// the `true` gives fish the "__fish_use_subcommand" behavior
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			case internalShell.PowerShell:
				return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				cmd.SilenceUsage = true
				return fmt.Errorf("Unsupported shell %q.", args[0])
			}
		},
	}

	// Add subcommands
	cmd.AddCommand(
		install.Cmd(),
		uninstall.Cmd(),
	)

	return cmd
}
