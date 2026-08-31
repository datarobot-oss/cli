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

package idargs

import (
	"errors"
	"fmt"
	"os"

	"github.com/datarobot/cli/cmd/helpers"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// AddYesFlag registers --yes for the commands that ask before acting.
//
// Only the environment variable is bound to viper: an explicit --yes read
// through viper would land in AllSettings() and persist to drconfig.yaml.
func AddYesFlag(cmd *cobra.Command, usage string) {
	cmd.Flags().BoolP(cli.YesFlagName, "y", false, usage)

	_ = viperx.BindEnv(cli.YesFlagName, reader.NonInteractiveEnv)
}

// EnvConsent says whether the non-interactive environment variable may stand
// in for an answer.
//
// It may not when the target was specified in a manifest rather than on the
// command line and the operation cannot be undone. That variable is set once in
// CI to keep wizards from blocking a pipeline; letting it also mean "yes,
// delete whatever the nearest manifest points at" turns a convenience into
// standing consent for something nobody named. An explicit --yes is still
// accepted, because that is typed for this command.
type EnvConsent bool

const (
	EnvMayConsent    EnvConsent = true
	EnvMayNotConsent EnvConsent = false
)

// Confirm puts the question, unless it has already been answered. A declined
// prompt is (false, nil) so the caller can exit 0 as a no-op.
//
// Whether to ask at all stays with the caller: `delete` always asks, while
// `stop` and `start` ask only when the workload was specified in a manifest
// rather than on the command line.
func Confirm(cmd *cobra.Command, question string, env EnvConsent) (bool, error) {
	yesFlag, _ := cmd.Flags().GetBool(cli.YesFlagName)
	if yesFlag {
		return true, nil
	}

	// Past the flag, cli.IsNonInteractive is exactly "the environment says so",
	// which is the half of automation consent an ambient delete refuses.
	declared := cli.IsNonInteractive(cmd)
	if declared && bool(env) {
		return true, nil
	}

	remedy := "pass --yes (or set " + reader.NonInteractiveEnv + "=1)"
	if !env {
		remedy = "pass --yes"
	}

	// A JSON run cannot be asked even on a terminal: the question would land
	// in the document something is parsing, and IsStdinTerminal cannot see
	// that, because it only watches stdin. `up` reaches the same conclusion
	// for the same reason. Said separately from the no-terminal case so the
	// reader learns which of the two is stopping them.
	if emitsJSON(cmd) {
		return false, errors.New(
			"--output-format json cannot ask for confirmation: " + remedy + " to run without a prompt")
	}

	// Declaring the environment non-interactive has to mean "do not ask", even
	// where it does not mean "yes". Reading it only as consent would leave the
	// one case that refuses it, an ambient delete, blocking on a prompt nobody
	// is there to answer.
	if declared || !canAsk(cmd) {
		return false, errors.New("confirmation required: " + remedy + " to run without a prompt")
	}

	// The question goes to stderr, not stdout: stdout is the acknowledgement
	// these commands promise, or one JSON document, and a question printed
	// into either breaks whatever is reading it.
	confirmed, err := helpers.Confirm(cmd.ErrOrStderr(), cmd.InOrStdin(), question)
	if err != nil || confirmed {
		return confirmed, err
	}

	// Said once here rather than at three call sites, so a decline never looks
	// like the command silently doing nothing.
	fmt.Fprintln(cmd.ErrOrStderr(), tui.DimStyle.Render("Aborted."))

	return false, nil
}

// Prompt is the question a mutating command asks before it acts. consequence
// is what agreeing costs, empty for a verb that can be undone.
//
// One skeleton for all three verbs, so they cannot come to ask about the same
// kind of target in three shapes. The id is given whole, because an id the
// reader has never seen is nothing to compare against, and Provenance says why
// the question is being put rather than leaving that to be inferred. Both the
// provenance and the consequence carry their own trailing space, so an absent
// clause leaves no double space behind.
func Prompt(verb string, ref Ref, consequence string) string {
	if consequence != "" {
		consequence += " "
	}

	return fmt.Sprintf("%s workload %s? %s%s[y/N] ", verb, ref.ID, ref.Provenance(), consequence)
}

// canAsk reports whether there is a terminal on both ends of the question.
//
// stdout is not consulted: the answer is read from stdin and the question is
// written to stderr, and it is those two that have to be a terminal. Checking
// stdin alone was enough while the question went to stdout; now that a
// redirected stderr can swallow it, a prompt nobody can see is a prompt that
// hangs.
func canAsk(cmd *cobra.Command) bool {
	if !reader.IsStdinTerminal() {
		return false
	}

	f, ok := cmd.ErrOrStderr().(*os.File)
	if !ok {
		// Not a file at all, which in practice means a test buffer. The
		// question is capturable, so it can be asked.
		return true
	}

	return term.IsTerminal(int(f.Fd())) //nolint:gosec // uintptr and int are same size on supported platforms
}
