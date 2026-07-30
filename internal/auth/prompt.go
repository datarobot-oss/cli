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
	"github.com/charmbracelet/lipgloss"
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
)

// SpinnerLabel returns the label for the wait spinner.
//
// Only the opened case claims a browser is opening; the others would be lying.
func SpinnerLabel(state BrowserState) string {
	if state == BrowserOpened {
		return openingBrowserLabel
	}

	return awaitingAuthLabel
}

// RenderBrowserPrompt returns the block shown beneath the spinner while the CLI
// waits for the browser callback.
//
// When the browser opened the link is a quiet fallback. Otherwise it is promoted
// into a bordered box, because it is then the only way for the user to continue.
func RenderBrowserPrompt(authURL string, state BrowserState) string {
	link := tui.InfoStyle.Underline(true).Render(authURL)

	if state == BrowserOpened {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			"",
			tui.HintStyle.Render(browserOpenedHint),
			link,
			"",
		)
	}

	lines := []string{""}

	// A skipped browser is expected, so it gets no warning line.
	if state == BrowserFailed {
		lines = append(lines, tui.ErrorStyle.Render(browserFailedHint))
	}

	lines = append(lines,
		tui.BaseTextStyle.Render(browserManualHint),
		"",
		tui.NoteBoxStyle.Render(link),
		"",
	)

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}
