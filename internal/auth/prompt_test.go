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
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const testAuthURL = "https://app.datarobot.com/account/developer-tools?cliRedirect=true"

func TestBrowserStateFor(t *testing.T) {
	assert.Equal(t, BrowserOpened, BrowserStateFor(nil))
	assert.Equal(t, BrowserFailed, BrowserStateFor(errors.New("xdg-open: not found")))
}

func TestSpinnerLabel_OnlyClaimsOpeningWhenItActuallyOpened(t *testing.T) {
	// Saying "Opening your browser" when no browser opened is the confusing part of
	// CFX-6318, just inverted - it must not survive into the failure paths.
	assert.Equal(t, openingBrowserLabel, SpinnerLabel(BrowserOpened))
	assert.Equal(t, awaitingAuthLabel, SpinnerLabel(BrowserFailed))
	assert.Equal(t, awaitingAuthLabel, SpinnerLabel(BrowserSkipped))
}

func TestRenderBrowserPrompt_OpenedKeepsLinkAsQuietFallback(t *testing.T) {
	out := RenderBrowserPrompt(testAuthURL, BrowserOpened)

	assert.Contains(t, stripANSI(out), browserOpenedHint)
	assert.Contains(t, stripANSI(out), testAuthURL)

	// No apology and no box: the browser did open, so the link is a footnote.
	assert.NotContains(t, stripANSI(out), browserFailedHint)
	assert.NotContains(t, out, "╭", "the link must not be boxed when the browser opened")
}

func TestRenderBrowserPrompt_FailedPromotesLinkAndSaysSo(t *testing.T) {
	out := RenderBrowserPrompt(testAuthURL, BrowserFailed)
	plain := stripANSI(out)

	assert.Contains(t, plain, browserFailedHint, "the user must be told the browser did not open")
	assert.Contains(t, plain, browserManualHint)
	assert.Contains(t, plain, testAuthURL)
	assert.Contains(t, out, "╭", "the link becomes the primary instruction and is boxed")
}

func TestRenderBrowserPrompt_SkippedPromotesLinkWithoutWarning(t *testing.T) {
	// --no-browser is a deliberate choice, so it must not be reported as a failure.
	out := RenderBrowserPrompt(testAuthURL, BrowserSkipped)
	plain := stripANSI(out)

	assert.NotContains(t, plain, browserFailedHint)
	assert.Contains(t, plain, browserManualHint)
	assert.Contains(t, plain, testAuthURL)
	assert.Contains(t, out, "╭")
}

func TestRenderBrowserPrompt_LabelAndPromptComposeIntoMultilineSpinnerLabel(t *testing.T) {
	// RunBrowserLoginWith concatenates these two and hands the result to
	// tui.RunWithSpinner, which renders "<spinner> <label>". The prompt therefore
	// has to begin on its own line or it would be glued to the spinner text.
	label := SpinnerLabel(BrowserOpened) + RenderBrowserPrompt(testAuthURL, BrowserOpened)
	lines := strings.Split(stripANSI(label), "\n")

	// lipgloss pads every joined line to the width of the widest one, so compare
	// content rather than the incidental trailing whitespace.
	assert.Equal(t, openingBrowserLabel, strings.TrimRight(lines[0], " "),
		"the spinner's own line must contain only the label")
	assert.Greater(t, len(lines), 2, "the fallback link must render below the spinner line")
	assert.Contains(t, stripANSI(label), testAuthURL)
}

// stripANSI removes lipgloss colour escapes so assertions can match plain text.
// Styling is environment dependent (it varies with terminal colour support), so
// tests assert on content, never on the escape sequences.
func stripANSI(s string) string {
	var (
		out    strings.Builder
		inEsc  bool
		escSeq strings.Builder
	)

	for _, r := range s {
		switch {
		case r == 0x1b:
			inEsc = true

			escSeq.Reset()
		case inEsc:
			escSeq.WriteRune(r)
			// CSI sequences end with a byte in the range @ to ~.
			if r >= '@' && r <= '~' && escSeq.Len() > 1 {
				inEsc = false
			}
		default:
			out.WriteRune(r)
		}
	}

	return out.String()
}
