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

package up

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/datarobot/cli/tui"
)

func renderLines(t *testing.T, lines []diffLine, redact func(string) bool) string {
	t.Helper()

	var b strings.Builder

	require.NoError(t, renderUnified(&b, lines, redact))

	return b.String()
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(text string) string {
	return ansiPattern.ReplaceAllString(text, "")
}

func ctxLine(text string) diffLine {
	return diffLine{kind: lineContext, path: text, text: text}
}

func addLine(text string) diffLine {
	return diffLine{kind: lineAdd, path: text, text: text}
}

func delLine(text string) diffLine {
	return diffLine{kind: lineDel, path: text, text: text}
}

// Three lines either side of a change are kept; a run longer than that
// collapses into one counted line.
func TestRenderUnified_ContextWindowIsThreeEachSide(t *testing.T) {
	lines := []diffLine{
		ctxLine("a"), ctxLine("b"), ctxLine("c"), ctxLine("d"), ctxLine("e"),
		delLine("old"), addLine("new"),
		ctxLine("f"), ctxLine("g"), ctxLine("h"), ctxLine("i"), ctxLine("j"),
	}

	out := stripANSI(renderLines(t, lines, nil))

	assert.Equal(t, strings.Join([]string{
		"... 2 identical lines",
		"c", "d", "e",
		"- old", "+ new",
		"f", "g", "h",
		"... 2 identical lines",
		"",
	}, "\n"), out)
}

// A run of exactly the window's width is shown in full; one past it
// collapses, because a line that says "1 identical lines" would be longer
// than the line it hides.
func TestRenderUnified_RunAtTheWindowIsShownInFull(t *testing.T) {
	atWindow := []diffLine{ctxLine("a"), ctxLine("b"), ctxLine("c"), addLine("new")}

	assert.Equal(t, "a\nb\nc\n+ new\n", stripANSI(renderLines(t, atWindow, nil)))

	past := []diffLine{ctxLine("a"), ctxLine("b"), ctxLine("c"), ctxLine("d"), addLine("new")}

	assert.Equal(t, "... 1 identical lines\nb\nc\nd\n+ new\n", stripANSI(renderLines(t, past, nil)))
}

// Seven context lines between two changes keep three on each side and hide
// the one in the middle.
func TestRenderUnified_SevenBetweenTwoChangesHidesTheMiddleOne(t *testing.T) {
	lines := make([]diffLine, 0, 9)
	lines = append(lines, addLine("x"))

	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		lines = append(lines, ctxLine(name))
	}

	lines = append(lines, delLine("y"))

	out := stripANSI(renderLines(t, lines, nil))

	assert.Contains(t, out, "c\n... 1 identical lines\ne\n")
}

func TestRenderUnified_AllContextCollapsesToOneLine(t *testing.T) {
	lines := []diffLine{ctxLine("a"), ctxLine("b"), ctxLine("c"), ctxLine("d")}

	assert.Equal(t, "... 4 identical lines\n", stripANSI(renderLines(t, lines, nil)))
}

func TestRenderUnified_NoLinesWritesNothing(t *testing.T) {
	assert.Empty(t, renderLines(t, nil, nil))
}

// A redacted path prints its placeholder whatever the kind, and the value
// in the text never reaches the output.
func TestRenderUnified_RedactionAppliesToEveryKind(t *testing.T) {
	secret := "sk-plaintext"
	redact := func(path string) bool { return path == "env[KEY]" }

	cases := []struct {
		name        string
		line        diffLine
		placeholder string
	}{
		{"add", diffLine{kind: lineAdd, path: "env[KEY]", text: "env[KEY]: " + secret}, setPlaceholder},
		{"del", diffLine{kind: lineDel, path: "env[KEY]", text: "env[KEY]: " + secret}, changedPlaceholder},
		{"removed", diffLine{kind: lineDel, path: "env[KEY]", text: "env[KEY]: " + secret, removed: true}, removedPlaceholder},
		{"context", diffLine{kind: lineContext, path: "env[KEY]", text: "env[KEY]: " + secret}, hiddenPlaceholder},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// A change beside the line keeps a context line inside the window.
			out := stripANSI(renderLines(t, []diffLine{c.line, addLine("other")}, redact))

			assert.NotContains(t, out, secret)
			assert.Contains(t, out, "env[KEY]: "+c.placeholder)
		})
	}
}

// The three kinds wear the plan's palette: additions as success, removals
// as warning, context as hint.
func TestRenderUnified_PerKindStylesWithForcedColorProfile(t *testing.T) {
	prev := lipgloss.ColorProfile()

	lipgloss.SetColorProfile(termenv.ANSI)

	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	out := renderLines(t, []diffLine{ctxLine("same"), addLine("new"), delLine("old")}, nil)

	assert.Contains(t, out, tui.SuccessStyle.Render("+ new"))
	assert.Contains(t, out, tui.WarnStyle.Render("- old"))
	assert.Contains(t, out, tui.HintStyle.Render("same"))
	assert.Equal(t, tui.SuccessStyle, lineStyle(lineAdd))
	assert.Equal(t, tui.WarnStyle, lineStyle(lineDel))
	assert.Equal(t, tui.HintStyle, lineStyle(lineContext))
}

func TestRenderUnified_RedactHookIsConsultedOncePerShownLine(t *testing.T) {
	calls := 0
	redact := func(string) bool { calls++; return false }

	renderLines(t, []diffLine{ctxLine("a"), addLine("b"), delLine("c"), ctxLine("d")}, redact)

	assert.Equal(t, 4, calls)
}

func TestRenderUnified_PropagatesWriteErrors(t *testing.T) {
	err := renderUnified(&failingWriter{err: errors.New("write failed")}, []diffLine{addLine("x")}, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot write diff")
}
