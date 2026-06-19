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

package checkout

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/datarobot/cli/cmd/artifact/code/internal/dirprompt"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/spf13/cobra"
)

type Deps struct {
	GetArtifact   func(string) (*workload.Artifact, error)
	Files         filesapi.Client
	PromptDir     dirprompt.PromptFunc
	PromptVersion dirprompt.PromptNoDefaultFunc
}

func defaultDeps() Deps {
	return Deps{
		GetArtifact:   workload.GetArtifact,
		Files:         filesapi.New(),
		PromptDir:     dirprompt.AskWithDefault,
		PromptVersion: dirprompt.Ask,
	}
}

func init() {
	// Per project rules: bind only the env var, read the flag directly from cobra.
	_ = viperx.BindEnv("yes", "DATAROBOT_CLI_NON_INTERACTIVE")
}

func Cmd() *cobra.Command {
	return cmdWithDeps(defaultDeps())
}

func cmdWithDeps(deps Deps) *cobra.Command {
	var outputFormat outputformat.OutputFormat

	c := &cobra.Command{
		Use:          "checkout [<ver>]",
		Short:        "Download a version snapshot for read-only inspection.",
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		Long: `Download a specific catalog version into '.wapi/.checkouts/<version-id>/'
for read-only inspection. The working directory and '.wapi/' sync state
are never modified.

The version argument may be a full version ID or any unique prefix.
If omitted (and --yes is not set), you will be prompted.

With --clean, remove existing checkout directories instead of downloading:
no positional argument removes all checkouts; a positional argument
removes only the matching one.

Run 'dr artifact code init <artifact-id>' first to link a project
directory to an artifact.

Example:
  dr artifact code checkout
  dr artifact code checkout abcdef12
  dr artifact code checkout abcdef12 --dir ./service
  dr artifact code checkout --clean
  dr artifact code checkout abcdef12 --clean`,
		PreRunE: auth.EnsureAuthenticatedE,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			return runCheckout(cmd, args, outputFormat, deps)
		},
	}

	outputformat.AddFlag(c, &outputFormat)

	c.Flags().String("dir", "", "Project directory (default: current directory).")
	c.Flags().Bool("clean", false, "Remove checkout directories instead of downloading.")
	c.Flags().BoolP("yes", "y", false, "Skip interactive prompts.")

	return c
}

func runCheckout(cmd *cobra.Command, args []string, outputFormat outputformat.OutputFormat, deps Deps) error {
	yesFlag, _ := cmd.Flags().GetBool("yes")
	yes := yesFlag || viperx.GetBool("yes")

	dirFlag, _ := cmd.Flags().GetString("dir")
	clean, _ := cmd.Flags().GetBool("clean")

	dir, err := resolveProjectDir(dirFlag, yes, deps.PromptDir)
	if err != nil {
		return err
	}

	if !wapi.Exists(dir) {
		return errors.New("not linked to an artifact. Run 'dr artifact code init <id>' first")
	}

	if clean {
		var arg string

		if len(args) == 1 {
			arg = args[0]
		}

		return runClean(cmd.OutOrStdout(), outputFormat, dir, arg)
	}

	verArg, err := resolveVersionArg(cmd.ErrOrStderr(), args, yes, deps.PromptVersion)
	if err != nil {
		return err
	}

	return runDownload(cmd.OutOrStdout(), outputFormat, dir, verArg, deps)
}

func resolveProjectDir(dirFlag string, yes bool, prompt dirprompt.PromptFunc) (string, error) {
	dir, err := dirprompt.ResolveDir(dirFlag, yes, prompt)
	if err != nil {
		return "", err
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve dir %s: %w", dir, err)
	}

	return abs, nil
}

func resolveVersionArg(stderr io.Writer, args []string, yes bool, prompt dirprompt.PromptNoDefaultFunc) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}

	if yes {
		return "", errors.New("a version argument is required (or pass --clean to remove checkouts)")
	}

	fmt.Fprintln(stderr, "Run 'dr artifact code versions' to list available versions.")

	return prompt("Code version ID")
}
