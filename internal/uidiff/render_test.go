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

package uidiff

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renderRows runs Render against a buffer, which is how every test below
// reads a rendering.
func renderRows(t *testing.T, rows []Row, opts Options) string {
	t.Helper()

	var b strings.Builder

	require.NoError(t, Render(&b, rows, opts))

	return b.String()
}

// stripANSI removes lipgloss colour escapes so assertions can match plain
// text. Styling is environment dependent (it varies with terminal colour
// support), so tests assert on content, never on the escape sequences. That
// is also the parseability guarantee: with styles stripped the markers are
// still there, because they are ordinary characters the styling wrapped.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(text string) string {
	return ansiPattern.ReplaceAllString(text, "")
}

// ctxRow, addRow and delRow build rows whose visible text is the argument
// itself, so an expected rendering reads as the rows it came from.
func ctxRow(text string) Row {
	return Row{Kind: Context, Path: text, Text: text}
}

func addRow(text string) Row {
	return Row{Kind: Add, Path: text, Text: text}
}

func delRow(text string) Row {
	return Row{Kind: Del, Path: text, Text: text}
}

// extraRow is an unmanaged row the way the workload caller emits them: the
// leading marker and the wording are the caller's, carried in Text, and the
// renderer adds nothing of its own.
func extraRow(path string) Row {
	return Row{Kind: Unmanaged, Path: path, Text: "~ " + path + ": not managed by this file"}
}

// VAL-DIFF-001: the window is exactly three unchanged rows either side of a
// change, and everything beyond it on both sides collapses.
func TestRender_ContextWindowIsThreeEachSide(t *testing.T) {
	rows := []Row{
		ctxRow("before-1"), ctxRow("before-2"), ctxRow("before-3"),
		ctxRow("before-4"), ctxRow("before-5"), ctxRow("before-6"),
		addRow("the change"),
		ctxRow("after-1"), ctxRow("after-2"), ctxRow("after-3"),
		ctxRow("after-4"), ctxRow("after-5"), ctxRow("after-6"),
	}

	out := renderRows(t, rows, Options{})

	assert.Equal(t,
		"... 3 identical lines\n"+
			"before-4\nbefore-5\nbefore-6\n"+
			"+ the change\n"+
			"after-1\nafter-2\nafter-3\n"+
			"... 3 identical lines\n",
		stripANSI(out))
}

// A side that cannot fill the window shows what is there rather than padding.
func TestRender_ChangeWithFewerContextRowsShowsWhatIsThere(t *testing.T) {
	rows := []Row{
		ctxRow("only-1"), ctxRow("only-2"),
		delRow("the change"),
		ctxRow("after"),
	}

	out := renderRows(t, rows, Options{})

	assert.Equal(t,
		"only-1\nonly-2\n- the change\nafter\n",
		stripANSI(out))
}

// VAL-DIFF-002: a run at exactly the window shows in full, no collapse line.
func TestRender_RunAtTheWindowIsShownInFull(t *testing.T) {
	rows := []Row{
		addRow("first change"),
		ctxRow("between-1"), ctxRow("between-2"), ctxRow("between-3"),
		delRow("second change"),
	}

	out := stripANSI(renderRows(t, rows, Options{}))

	assert.Equal(t,
		"+ first change\nbetween-1\nbetween-2\nbetween-3\n- second change\n",
		out)
	assert.NotContains(t, out, "...", "a run at the window never collapses")
}

// One row past the window: the three change-adjacent rows survive and the
// single farthest one collapses, counted exactly.
func TestRender_RunOnePastTheWindowCollapses(t *testing.T) {
	rows := []Row{
		ctxRow("far"), ctxRow("near-1"), ctxRow("near-2"), ctxRow("near-3"),
		addRow("the change"),
	}

	out := renderRows(t, rows, Options{})

	assert.Equal(t,
		"... 1 identical lines\nnear-1\nnear-2\nnear-3\n+ the change\n",
		stripANSI(out))
}

// Two changes seven rows apart: each keeps its three, and the one middle row
// neither side reaches is the whole hidden count.
func TestRender_SevenBetweenTwoChangesHidesTheMiddleOne(t *testing.T) {
	rows := []Row{
		addRow("first"),
		ctxRow("c1"), ctxRow("c2"), ctxRow("c3"),
		ctxRow("c4"), ctxRow("c5"), ctxRow("c6"), ctxRow("c7"),
		delRow("second"),
	}

	out := renderRows(t, rows, Options{})

	assert.Equal(t,
		"+ first\nc1\nc2\nc3\n... 1 identical lines\nc5\nc6\nc7\n- second\n",
		stripANSI(out))
}

// Two long runs in one body collapse separately, each line counting only its
// own run.
func TestRender_TwoLongRunsCollapseIndependently(t *testing.T) {
	rows := []Row{
		ctxRow("lead-1"), ctxRow("lead-2"), ctxRow("lead-3"),
		ctxRow("lead-4"), ctxRow("lead-5"),
		addRow("the change"),
		ctxRow("tail-1"), ctxRow("tail-2"), ctxRow("tail-3"),
		ctxRow("tail-4"), ctxRow("tail-5"),
	}

	out := renderRows(t, rows, Options{})

	assert.Equal(t,
		"... 2 identical lines\nlead-3\nlead-4\nlead-5\n"+
			"+ the change\n"+
			"tail-1\ntail-2\ntail-3\n... 2 identical lines\n",
		stripANSI(out))
}

// VAL-DIFF-003: runs touching the ends of the list collapse away from the
// change, on the far side of the rows that survive.
func TestRender_ListEdgeRunsCollapseAwayFromTheChange(t *testing.T) {
	leading := renderRows(t, []Row{
		ctxRow("c1"), ctxRow("c2"), ctxRow("c3"), ctxRow("c4"), ctxRow("c5"),
		addRow("the change"),
	}, Options{})

	assert.Equal(t,
		"... 2 identical lines\nc3\nc4\nc5\n+ the change\n",
		stripANSI(leading),
		"the collapse line sits on the far side of a leading run, never the change side")

	trailing := renderRows(t, []Row{
		delRow("the change"),
		ctxRow("c1"), ctxRow("c2"), ctxRow("c3"), ctxRow("c4"), ctxRow("c5"),
	}, Options{})

	assert.Equal(t,
		"- the change\nc1\nc2\nc3\n... 2 identical lines\n",
		stripANSI(trailing))
}

// A body with no change at all is one whole hidden run: the collapse line is
// the only output, and it counts everything.
func TestRender_AllContextCollapsesToOneLine(t *testing.T) {
	out := renderRows(t, []Row{
		ctxRow("c1"), ctxRow("c2"), ctxRow("c3"), ctxRow("c4"),
	}, Options{})

	assert.Equal(t, "... 4 identical lines\n", stripANSI(out))
}

// VAL-DIFF-005: unmanaged rows run as context, carry the caller's marker
// from Text, and never render as a removal.
func TestRender_UnmanagedRowsRunWithContextAndNeverRenderAsRemovals(t *testing.T) {
	rows := []Row{
		ctxRow("c1"), ctxRow("c2"),
		extraRow("environmentVars[SIDECAR]"),
		extraRow("environmentVars[AUTOSCALE]"),
		addRow("runtime.replicaCount: 3"),
	}

	out := renderRows(t, rows, Options{})

	assert.Equal(t,
		"... 1 identical lines\nc2\n"+
			"~ environmentVars[SIDECAR]: not managed by this file\n"+
			"~ environmentVars[AUTOSCALE]: not managed by this file\n"+
			"+ runtime.replicaCount: 3\n",
		stripANSI(out))
}

// The markers are the vocabulary the plan block already prints, so a diff
// and the default plan read as one visual language.
func TestRender_MarkersFollowThePlanVocabulary(t *testing.T) {
	rows := []Row{
		ctxRow("unchanged"),
		delRow("runtime.replicaCount: 1"),
		addRow("runtime.replicaCount: 3"),
		extraRow("environmentVars[SIDECAR]"),
	}

	out := renderRows(t, rows, Options{})

	assert.Equal(t,
		"unchanged\n"+
			"- runtime.replicaCount: 1\n"+
			"+ runtime.replicaCount: 3\n"+
			"~ environmentVars[SIDECAR]: not managed by this file\n",
		stripANSI(out))
}

// VAL-DIFF-004: the redaction hook suppresses the text of every Kind it
// fires for, whatever the row was carrying, and never touches the others.
func TestRender_RedactionAppliesToEveryKind(t *testing.T) {
	redact := func(path string) bool {
		return strings.Contains(path, ".environmentVars[")
	}

	rows := []Row{
		ctxRow("port: 9090"),
		{Kind: Context, Path: "runtime.environmentVars[LOG_LEVEL]", Text: "runtime.environmentVars[LOG_LEVEL]: debug"},
		{Kind: Add, Path: "runtime.environmentVars[TOKEN]", Text: "runtime.environmentVars[TOKEN]: plaintext-secret"},
		{Kind: Del, Path: "runtime.environmentVars[OLD]", Text: "runtime.environmentVars[OLD]: dr-credential:abc123/client-secret"},
		{Kind: Unmanaged, Path: "runtime.environmentVars[SIDE]", Text: "~ runtime.environmentVars[SIDE]: not managed by this file"},
		addRow("port: 9091"),
	}

	out := stripANSI(renderRows(t, rows, Options{Redact: redact}))

	for _, secret := range []string{
		"plaintext-secret",
		"dr-credential:abc123/client-secret",
		"debug",
		"not managed by this file",
	} {
		assert.NotContains(t, out, secret)
	}

	assert.Contains(t, out, "runtime.environmentVars[LOG_LEVEL]: (redacted)")
	assert.Contains(t, out, "+ runtime.environmentVars[TOKEN]: set")
	assert.Contains(t, out, "- runtime.environmentVars[OLD]: changed")
	assert.Contains(t, out, "runtime.environmentVars[SIDE]: (redacted)")

	// Rows the hook does not fire for keep their values, so redaction is a
	// property of the path, not of the renderer.
	assert.Contains(t, out, "port: 9090")
	assert.Contains(t, out, "+ port: 9091")
}

// Without a hook nothing is redacted; the placeholder machinery is opt-in.
func TestRender_WithoutARedactHookTheValuesPrint(t *testing.T) {
	rows := []Row{
		{Kind: Add, Path: "runtime.environmentVars[TOKEN]", Text: "runtime.environmentVars[TOKEN]: secret"},
		addRow("port: 9091"),
	}

	out := renderRows(t, rows, Options{})

	assert.Contains(t, stripANSI(out), "+ runtime.environmentVars[TOKEN]: secret")
}

// The hook is a per-row decision: once per row, never per line of output.
func TestRender_RedactHookIsConsultedOncePerRow(t *testing.T) {
	calls := 0

	rows := []Row{ctxRow("a"), addRow("b"), delRow("c"), extraRow("d"), ctxRow("e")}

	var b strings.Builder

	require.NoError(t, Render(&b, rows, Options{Redact: func(string) bool {
		calls++

		return false
	}}))

	assert.Equal(t, len(rows), calls, "the hook runs once per row")
}

// VAL-DIFF-001: zero means the default window, and the Context field is how
// a caller asks for a different one.
func TestRender_ZeroContextMeansTheDefaultWindow(t *testing.T) {
	rows := []Row{
		ctxRow("c1"), ctxRow("c2"), ctxRow("c3"), ctxRow("c4"), ctxRow("c5"),
		addRow("the change"),
	}

	assert.Equal(t,
		renderRows(t, rows, Options{Context: 3}),
		renderRows(t, rows, Options{}),
		"zero and three are the same window")

	out := renderRows(t, rows, Options{Context: 1})

	assert.Equal(t, "... 4 identical lines\nc5\n+ the change\n", stripANSI(out))
}

// An empty body is an empty rendering, not a stray newline.
func TestRender_NoRowsWritesNothing(t *testing.T) {
	assert.Empty(t, renderRows(t, nil, Options{}))
}

var errWriteFailed = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWriteFailed }

// A writer that fails leaves the caller holding the error; a diff that
// reported success while losing lines would be worse than one that failed.
func TestRender_PropagatesWriteErrors(t *testing.T) {
	err := Render(failingWriter{}, []Row{ctxRow("c")}, Options{})

	require.ErrorIs(t, err, errWriteFailed)
}
