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
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every line, including the ones that arrive in the same call, because that is
// how the wizard writes a notice and its remedy.
func TestIndent_PutsTheMarginOnEveryLine(t *testing.T) {
	out := &strings.Builder{}

	fmt.Fprintf(indent(out, "  "), "first.\n  second.\n")
	fmt.Fprintf(indent(out, "  "), "third.\n")

	assert.Equal(t, "  first.\n    second.\n  third.\n", out.String())
}

// A margin with nothing after it is trailing whitespace, which the plan
// rendering is careful about for the same reason.
func TestIndent_LeavesABlankLineBlank(t *testing.T) {
	out := &strings.Builder{}

	fmt.Fprint(indent(out, "  "), "first.\n\nsecond.\n")

	assert.Equal(t, "  first.\n\n  second.\n", out.String())
}

// A line split across calls is one line, so the margin goes on once.
func TestIndent_DoesNotRepeatTheMarginMidLine(t *testing.T) {
	out := &strings.Builder{}
	w := indent(out, "  ")

	fmt.Fprint(w, "half a ")
	fmt.Fprint(w, "line\n")

	assert.Equal(t, "  half a line\n", out.String())
}

// failingWriter is a writer that will not take everything it is given, which
// is what a full disk and a closed pipe both look like from here.
type failingWriter struct {
	accept int
	err    error
	got    strings.Builder
}

func (f *failingWriter) Write(p []byte) (int, error) {
	if f.accept < len(p) {
		p = p[:f.accept]
	}

	f.got.Write(p)
	f.accept -= len(p)

	return len(p), f.err
}

// A failure has to reach the caller, and it must not leave the wrapper
// believing a line ended that never did: the next write would put the margin
// in the middle of it.
func TestIndent_ReportsAFailedWriteAndKeepsItsPlace(t *testing.T) {
	failing := &failingWriter{accept: 4, err: errors.New("disk full")}
	w := indent(failing, "  ")

	n, err := w.Write([]byte("first.\nsecond.\n"))
	require.Error(t, err)

	assert.Zero(t, n, "the margin makes the two lengths differ, so a partial count would be a guess")
	assert.False(t, w.atLineStart, "the line w received was never finished")
}

// A short write with no error is a writer breaking its own contract, and
// passing it on as success drops the tail of a warning with nothing to say so.
func TestIndent_TurnsAShortWriteIntoAnError(t *testing.T) {
	short := &failingWriter{accept: 3}
	_, err := indent(short, "  ").Write([]byte("first.\n"))

	require.ErrorIs(t, err, io.ErrShortWrite)
}

// What the caller asked for, not what the margin added: fmt reads a smaller
// count as a short write.
func TestIndent_ReportsTheCallersOwnLength(t *testing.T) {
	out := &strings.Builder{}

	n, err := indent(out, "  ").Write([]byte("line\n"))
	require.NoError(t, err)

	assert.Equal(t, len("line\n"), n)
}
