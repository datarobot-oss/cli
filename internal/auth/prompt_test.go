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

func TestRenderBrowserPrompt_OpenedFramesLinkAsQuietFallback(t *testing.T) {
	out := RenderBrowserPrompt(testAuthURL, BrowserOpened)
	plain := stripANSI(out)

	assert.Contains(t, plain, browserOpenedHint)
	assert.Contains(t, plain, testAuthURL)
	assert.Contains(t, plain, "╭", "the link is boxed in every state so the paths look alike")

	// The browser did open, so there is nothing to apologise for.
	assert.NotContains(t, plain, browserFailedHint)
}

func TestRenderBrowserPrompt_FailedPromotesLinkAndSaysSo(t *testing.T) {
	out := RenderBrowserPrompt(testAuthURL, BrowserFailed)
	plain := stripANSI(out)

	assert.Contains(t, plain, browserFailedHint, "the user must be told the browser did not open")
	assert.Contains(t, plain, browserManualHint)
	assert.Contains(t, plain, testAuthURL)
	assert.Contains(t, plain, "╭")
}

func TestRenderBrowserPrompt_SkippedPromotesLinkWithoutWarning(t *testing.T) {
	// --no-browser is a deliberate choice, so it must not be reported as a failure.
	out := RenderBrowserPrompt(testAuthURL, BrowserSkipped)
	plain := stripANSI(out)

	assert.NotContains(t, plain, browserFailedHint)
	assert.Contains(t, plain, browserManualHint)
	assert.Contains(t, plain, testAuthURL)
	assert.Contains(t, plain, "╭")
}

// TestRenderBrowserPrompt_AllStatesShareOneShape is the consistency guard: the
// regular and --no-browser paths must be recognisably the same command, differing
// only in wording. It compares the structure with the text stripped out.
func TestRenderBrowserPrompt_AllStatesShareOneShape(t *testing.T) {
	shapes := map[BrowserState]string{}

	for _, state := range []BrowserState{BrowserOpened, BrowserFailed, BrowserSkipped} {
		plain := stripANSI(RenderBrowserPrompt(testAuthURL, state))

		// Every state: leading blank line, hint, blank line, three box lines.
		assert.True(t, strings.HasPrefix(plain, "\n\n"),
			"state %d must start with a blank line separating it from the spinner label", state)
		assert.True(t, strings.HasSuffix(plain, "\n"),
			"state %d must end with a newline", state)

		var boxLines int

		for line := range strings.SplitSeq(plain, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}

			// Non-blank lines are indented as a block.
			assert.True(t, strings.HasPrefix(line, strings.Repeat(" ", promptIndent)),
				"state %d line %q must be indented by %d spaces", state, line, promptIndent)

			if strings.ContainsAny(trimmed, "╭│╰") {
				boxLines++
			}
		}

		assert.Equal(t, 3, boxLines, "state %d must render a three-line bordered box", state)

		shapes[state] = strings.Join(boxLinesOf(plain), "\n")
	}

	// The box itself is identical across states; only the hint above it differs.
	assert.Equal(t, shapes[BrowserOpened], shapes[BrowserFailed])
	assert.Equal(t, shapes[BrowserOpened], shapes[BrowserSkipped])
}

// TestRenderBrowserPrompt_URLStaysContiguousInRawOutput guards a subtle rendering
// trap: styling text that lipgloss then wraps in a border makes it re-emit the
// content one character at a time with escape sequences in between. The URL then
// still *looks* right but is no longer a contiguous string, so terminals stop
// linkifying it and anything matching on raw output - notably
// smoke_test_scripts/expect_auth_login.exp, which waits for "cliRedirect=true" -
// silently stops finding it.
func TestRenderBrowserPrompt_URLStaysContiguousInRawOutput(t *testing.T) {
	for _, state := range []BrowserState{BrowserOpened, BrowserFailed, BrowserSkipped} {
		raw := RenderBrowserPrompt(testAuthURL, state)

		assert.Contains(t, raw, testAuthURL,
			"state %d: the URL must appear un-escaped in the raw output", state)
		assert.Contains(t, raw, "cliRedirect=true",
			"state %d: the smoke tests match this substring in raw terminal output", state)
	}
}

// boxLinesOf returns just the bordered-box lines of a rendered prompt.
func boxLinesOf(plain string) []string {
	var out []string

	for line := range strings.SplitSeq(plain, "\n") {
		if strings.ContainsAny(line, "╭│╰") {
			out = append(out, line)
		}
	}

	return out
}

func TestRenderBrowserPrompt_LabelAndPromptComposeIntoMultilineSpinnerLabel(t *testing.T) {
	// RunBrowserLoginWith concatenates these two and hands the result to
	// tui.RunWithSpinner, which renders "<spinner> <label>". The prompt therefore
	// has to begin on its own line or it would be glued to the spinner text.
	label := SpinnerLabel(BrowserOpened) + RenderBrowserPrompt(testAuthURL, BrowserOpened)
	lines := strings.Split(stripANSI(label), "\n")

	assert.Equal(t, openingBrowserLabel, lines[0],
		"the spinner's own line must contain only the label, with no padding")
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
