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

// root_helpers.go contains standalone helpers used by RootFactory that would
// cause import cycles if they lived inside root_factory.go:
//
//   - showFirstRunAnimation: displays the one-time DataRobot logo animation on
//     a user's first CLI invocation. Injected into the factory as AnimationFunc
//     so tests can replace it with a no-op.
//   - setUnknownArgGuards: walks the command tree after registration and installs
//     an Args validator + RunE on every pure-parent command so unrecognised
//     positional arguments produce a clear error instead of silently showing help.
package cmd

import (
	"fmt"
	"os"

	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/state"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

// showFirstRunAnimation displays the animated DataRobot logo inline (no
// alt-screen) on the very first CLI invocation, so the final settled frame
// remains in normal terminal scrollback and whatever runs next continues
// directly below it instead of hard-cutting from a full-screen takeover.
//
// It checks global state to avoid showing the animation more than once, only
// runs in interactive terminals, and is skipped entirely under automation
// (e.g. tools like expect that attach a real pty and would otherwise see
// animation frames mixed into output they are pattern-matching).
func showFirstRunAnimation() {
	if reader.IsNonInteractive() {
		return
	}

	if !state.IsFirstRun() {
		return
	}

	// Only show the animation when stdout is a terminal (not piped or redirected).
	fi, err := os.Stdout.Stat()
	if err != nil || (fi.Mode()&os.ModeCharDevice) == 0 {
		return
	}

	m := tui.NewLogoAnimationModel()

	_, _ = tui.Run(m)

	state.MarkAnimationShown()
}

// setUnknownArgGuards walks the command tree and installs a RunE + Args
// validator on every pure-parent command (has subcommands, no Run/RunE, no
// explicit Args set).
//
// Cobra checks !Runnable() before ValidateArgs, so a non-runnable parent
// silently shows help for any arg. Making the command runnable lets Args fire
// first and return a clear error; the RunE fallback shows help when no args
// are given.
func setUnknownArgGuards(root *cobra.Command) {
	for _, cmd := range root.Commands() {
		setUnknownArgGuards(cmd)
	}

	if !root.HasSubCommands() {
		return
	}

	if root.Args != nil || root.RunE != nil || root.Run != nil {
		return
	}

	root.Args = func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return nil
		}

		return fmt.Errorf("unknown command: %s", args[0])
	}

	root.RunE = func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	}
}
