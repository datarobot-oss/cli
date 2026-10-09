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

package initcmd

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/datarobot/cli/cmd/artifact/code/internal/dirprompt"
	"github.com/datarobot/cli/internal/cli"
	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/outputformat"
	wldoctor "github.com/datarobot/cli/internal/workload/doctor"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/spf13/cobra"
)

// offerRelinkFn is the interactive relink offer. It prints the notice to w,
// asks whether the user wants to relink (default No), and if yes, takes the
// id init was given or prompts for one. Returns the new ID and nil if
// accepted; "" and nil if declined; "" and an error on read failure (Ctrl-C,
// EOF). Tests override this to simulate user input without a real terminal.
var offerRelinkFn = defaultOfferRelink

// makeRelinkConfirmFn builds the confirm function for RunRelink from the init
// offer. Tests override this to inject a fake confirm. The production
// implementation mirrors the doctor command's makeRelinkConfirm: interactive
// TTY shows a [y/N] prompt (empty Enter declines); non-interactive prints the
// warning and proceeds.
var makeRelinkConfirmFn = makeInitRelinkConfirm

// isInteractiveFn reports whether the init command should use interactive
// prompts for the relink offer. Production: not non-interactive AND stdin is
// a terminal. Tests override this to force the interactive path without a
// real terminal.
var isInteractiveFn = defaultIsInteractive

// runRelinkFn is the relink execution seam. Production delegates to
// wldoctor.RunRelink; tests override this to capture the propagated context
// and assert it equals cmd.Context() (not context.Background()).
var runRelinkFn = wldoctor.RunRelink

// defaultIsInteractive returns true when the command is interactive (not
// --yes and stdin is a TTY).
func defaultIsInteractive(cmd *cobra.Command) bool {
	return !cli.IsNonInteractive(cmd) && reader.IsStdinTerminal()
}

// goneOrMismatch is one already-linked abort: which artifact is linked, the
// id init was given (the relink target, when there is one), and why.
type goneOrMismatch struct {
	dir     string
	linked  string
	givenID string
	gone    bool
}

// handleGoneOrMismatch handles the gone-artifact (404) or catalog-mismatch
// branch of the already-linked check: interactive → offer to relink in place
// (the id init was given is the target, else prompt for one, then run the
// doctor --relink path incl. warn/confirm and safety gates); non-interactive →
// print guidance naming dr artifact code doctor --relink. No message advises
// deleting state.
func handleGoneOrMismatch(cmd *cobra.Command, dir string, cfg wapi.Config, givenID string, outputFormat outputformat.OutputFormat, gone bool) error {
	stderr := cmd.ErrOrStderr()

	// The id init was given is the relink target, unless it is the gone one.
	// On a catalog mismatch the same id is the repair: a relink re-pins the
	// catalog from the artifact's current codeRef.
	if gone && givenID == cfg.ArtifactID {
		givenID = ""
	}

	abort := goneOrMismatch{dir: dir, linked: cfg.ArtifactID, givenID: givenID, gone: gone}

	// Non-interactive (--yes or non-TTY): print guidance, abort.
	if !isInteractiveFn(cmd) {
		return reportGoneOrMismatchAbort(cmd, abort, outputFormat)
	}

	// Interactive: offer to relink in place.
	var notice string

	if gone {
		notice = fmt.Sprintf("Linked artifact %s was not found (deleted?).", cfg.ArtifactID)
	} else {
		notice = fmt.Sprintf("Linked artifact %s has a catalog id mismatch.", cfg.ArtifactID)
	}

	newID, offerErr := offerRelinkFn(stderr, notice, givenID)
	if offerErr != nil || newID == "" {
		// Declined or read error → abort with guidance.
		return reportGoneOrMismatchAbort(cmd, abort, outputFormat)
	}

	// Accepted: run the relink.
	return runRelinkFromInit(cmd, dir, cfg.ArtifactID, newID, outputFormat)
}

// reportGoneOrMismatchAbort prints the non-interactive guidance (or the JSON
// abort shape) and returns an error to drive exit 1.
func reportGoneOrMismatchAbort(cmd *cobra.Command, a goneOrMismatch, outputFormat outputformat.OutputFormat) error {
	stderr := cmd.ErrOrStderr()

	if outputFormat == outputformat.OutputFormatJSON {
		id := a.linked

		renderAlreadyLinkedJSON(cmd.OutOrStdout(), &id, relinkRemedy(a.dir, a.givenID))

		printGoneOrMismatchGuidance(stderr, a)

		cmd.SilenceErrors = true

		return cli.ErrSilent
	}

	printGoneOrMismatchGuidance(cmd.OutOrStdout(), a)

	return errors.New("init aborted: project already linked")
}

func printGoneOrMismatchGuidance(w io.Writer, a goneOrMismatch) {
	if a.gone {
		printGoneGuidance(w, a.dir, a.linked, a.givenID)

		return
	}

	printMismatchGuidance(w, a.dir, a.linked, a.givenID)
}

// runRelinkFromInit executes the relink operation from the init offer and
// renders the result. On success, stdout carries the relink result (text or
// JSON) and the command returns nil (exit 0). On abort, the error drives
// exit 1.
func runRelinkFromInit(cmd *cobra.Command, dir, oldID, newID string, outputFormat outputformat.OutputFormat) error {
	stderr := cmd.ErrOrStderr()

	actions, err := runRelinkFn(cmd.Context(), wldoctor.RelinkOptions{
		ProjectDir:    dir,
		NewArtifactID: newID,
		Store:         wldoctor.ArtifactGetterFunc(getArtifactFn),
		Confirm:       makeRelinkConfirmFn(cmd),
	})
	if errors.Is(err, wldoctor.ErrRelinkPartial) {
		return reportPartialRelink(cmd, dir, newID, err, actions, outputFormat)
	}

	if err != nil {
		// Relink aborted (404, locked, wrong type, lock held, declined,
		// API unreachable). No state content changed.
		if outputFormat == outputformat.OutputFormatJSON {
			id := oldID

			renderAlreadyLinkedJSON(cmd.OutOrStdout(), &id, wldoctor.RemedyRelink)

			printRelinkAbortReason(stderr, err, actions)

			cmd.SilenceErrors = true

			return cli.ErrSilent
		}

		printRelinkAbortReason(stderr, err, actions)

		return errors.New("init aborted: relink failed")
	}

	// Relink succeeded: render the result.
	if outputFormat == outputformat.OutputFormatJSON {
		renderRelinkJSON(cmd.OutOrStdout(), newID, actions)

		return nil
	}

	printRelinkSuccess(cmd.OutOrStdout(), dir, newID)

	return nil
}

// reportPartialRelink reports a relink that repointed config.json but failed a
// later write: the project now names newID, so that is the id reported, and
// the remedy is the --fix that rebuilds the rest from the new config.
func reportPartialRelink(cmd *cobra.Command, dir, newID string, err error, actions []core.Action, outputFormat outputformat.OutputFormat) error {
	stderr := cmd.ErrOrStderr()

	for _, a := range actions {
		fmt.Fprintln(stderr, a.Reason)
	}

	fmt.Fprintln(stderr, err)

	if outputFormat == outputformat.OutputFormatJSON {
		id := newID

		renderAlreadyLinkedJSON(cmd.OutOrStdout(), &id, "dr artifact code doctor --fix"+manifest.DirFlag(dir))

		cmd.SilenceErrors = true

		return cli.ErrSilent
	}

	return errors.New("init aborted: relink incomplete")
}

// printRelinkAbortReason prints the relink abort reason to stderr. For
// ErrRelinkAbort the actions array describes the reason; for other errors
// (e.g. ErrRelinkAPIUnreachable) the error itself is the message.
func printRelinkAbortReason(stderr io.Writer, err error, actions []core.Action) {
	if !errors.Is(err, wldoctor.ErrRelinkAbort) {
		fmt.Fprintln(stderr, err)

		return
	}

	if len(actions) > 0 {
		fmt.Fprintln(stderr, actions[0].Reason)
	}
}

// defaultOfferRelink is the production interactive relink offer. It prints
// the notice to w, asks whether the user wants to relink (default No), and if
// yes, prompts for the new artifact ID.
//
// An empty entry at the new-artifact-ID prompt (dirprompt.Ask returning "") is
// treated as a decline, not a re-prompt. This is the intended default-No UX:
// dirprompt.Ask's contract returns "" on empty Enter, and the caller
// (handleGoneOrMismatch) treats "" as "declined, abort with guidance" —
// consistent with the bespoke [y/N] confirm prompt where empty Enter also
// declines. Re-prompting would surprise users who pressed Enter expecting to
// cancel.
func defaultOfferRelink(w io.Writer, notice, givenID string) (string, error) {
	fmt.Fprintln(w, notice)

	if givenID != "" {
		fmt.Fprintf(w, "Relink to artifact %s? [y/N] ", givenID)
	} else {
		fmt.Fprint(w, "Relink to a new artifact? [y/N] ")
	}

	line, err := reader.ReadString()
	if err != nil {
		return "", err
	}

	answer := strings.TrimSpace(strings.ToLower(line))
	if answer != "y" && answer != "yes" {
		return "", nil // declined
	}

	if givenID != "" {
		return givenID, nil
	}

	return dirprompt.Ask("New artifact ID")
}

// makeInitRelinkConfirm builds the confirm function for the relink from the
// init offer. Interactive (TTY, no --yes): the warning and a [y/N] prompt go
// to stderr; only "y"/"yes" proceeds (empty Enter declines — this is the
// bespoke default-No prompt, NOT reader.AskYesNo). With --yes the warning is
// printed and the relink proceeds; with no terminal and no --yes it declines.
func makeInitRelinkConfirm(cmd *cobra.Command) wldoctor.RelinkConfirmFunc {
	nonInteractive := cli.IsNonInteractive(cmd)

	stderr := cmd.ErrOrStderr()

	return func(warning string) bool {
		fmt.Fprintln(stderr, warning)

		// --yes (or DATAROBOT_CLI_NON_INTERACTIVE) is explicit consent.
		if nonInteractive {
			return true
		}

		// No terminal and no --yes: nobody can answer, so decline rather than
		// rewrite config.json, as doctor --relink does.
		if !reader.IsStdinTerminal() {
			return false
		}

		fmt.Fprint(stderr, "Proceed? [y/N] ")

		line, err := reader.ReadString()
		if err != nil {
			return false
		}

		answer := strings.TrimSpace(strings.ToLower(line))

		return answer == "y" || answer == "yes"
	}
}
