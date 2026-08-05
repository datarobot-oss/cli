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

package auth

import (
	"strings"

	"github.com/datarobot/cli/tui"
)

// BrowserState describes what happened when the CLI tried to open the browser.
// It drives the wording of the wait prompt, which differs in kind between the
// three cases: a browser that opened needs only a quiet fallback link, one that
// failed needs an apology plus a prominent link, and one that was deliberately
// skipped needs the prominent link without the apology.
type BrowserState int

const (
	// BrowserOpened means the browser was launched successfully.
	BrowserOpened BrowserState = iota
	// BrowserFailed means the browser could not be launched.
	BrowserFailed
	// BrowserSkipped means the caller asked not to launch a browser (--no-browser).
	BrowserSkipped
)

// BrowserStateFor maps the error from opening a browser onto a BrowserState.
func BrowserStateFor(openErr error) BrowserState {
	if openErr != nil {
		return BrowserFailed
	}

	return BrowserOpened
}

// Labels shown while the CLI waits for the browser callback. The wording is
// deliberately browser-first: opening the browser is the action being taken, and
// the link is the fallback, not the instruction.
const (
	openingBrowserLabel = "Opening your browser to sign in to DataRobot…"
	awaitingAuthLabel   = "Waiting for authorization…"

	browserOpenedHint = "Didn't open? Use this link:"
	browserFailedHint = "⚠ Couldn't open your browser automatically."
	browserManualHint = "Open this link to finish signing in:"

	// promptIndent lines the prompt block up under the spinner's label rather than
	// against the left edge of the terminal.
	promptIndent = 2
)

// indentLines prefixes every line with n spaces.
//
// Done by hand rather than with a lipgloss padding style because that pads every
// line out to the width of the widest one, leaving trailing whitespace on the
// short hint lines that users would pick up when copying the link.
func indentLines(s string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")

	for i, line := range lines {
		if line == "" {
			continue
		}

		lines[i] = pad + line
	}

	return strings.Join(lines, "\n")
}

// SpinnerLabel returns the label for the wait spinner.
//
// Only the opened case claims a browser is opening; the others would be lying.
func SpinnerLabel(state BrowserState) string {
	if state == BrowserOpened {
		return openingBrowserLabel
	}

	return awaitingAuthLabel
}

// linkHint is the line introducing the link. It is a quiet aside when the browser
// opened and an instruction when it did not.
func linkHint(state BrowserState) string {
	if state == BrowserOpened {
		return tui.HintStyle.Render(browserOpenedHint)
	}

	return tui.BaseTextStyle.Render(browserManualHint)
}

// RenderBrowserPrompt returns the block shown beneath the spinner while the CLI
// waits for the browser callback.
//
// Every state uses the same shape - a hint line, a blank line, then the link in a
// bordered box - so that `dr auth login` and `dr auth login --no-browser` look
// like the same command. Only the wording changes with the state.
func RenderBrowserPrompt(authURL string, state BrowserState) string {
	rows := make([]string, 0, 4)

	// Only a genuine failure warrants a warning; --no-browser is a deliberate
	// choice and must not be reported as something going wrong.
	if state == BrowserFailed {
		rows = append(rows, tui.ErrorStyle.Render(browserFailedHint))
	}

	// The URL is deliberately unstyled. Styling text that lipgloss then wraps in a
	// border makes it re-emit the content one character at a time with escapes in
	// between, so the URL stops being a contiguous string: terminals no longer
	// linkify it, copying it can pick up escapes, and anything matching on the
	// output (the expect-based smoke tests) stops finding it. The border already
	// draws the eye, so the text needs no help.
	rows = append(rows,
		linkHint(state),
		"",
		tui.NoteBoxStyle.Render(authURL),
	)

	block := indentLines(strings.Join(rows, "\n"), promptIndent)

	// A blank line separates the block from the spinner label it is appended to,
	// and a trailing newline keeps it off whatever the caller prints next.
	return "\n\n" + block + "\n"
}
