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

package reader

import (
	"bufio"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadLine_UnixNewline(t *testing.T) {
	line, err := readLine(bufio.NewReader(strings.NewReader("yes\nrest")))

	require.NoError(t, err)
	assert.Equal(t, "yes", line)
}

func TestReadLine_BareCarriageReturn(t *testing.T) {
	// Simulates Windows raw console-mode Enter, which delivers a bare '\r'
	// with no paired '\n' (ENABLE_LINE_INPUT is off).
	line, err := readLine(bufio.NewReader(strings.NewReader("yes\r")))

	require.NoError(t, err)
	assert.Equal(t, "yes", line)
}

func TestReadLine_WindowsCRLF(t *testing.T) {
	line, err := readLine(bufio.NewReader(strings.NewReader("yes\r\nrest")))

	require.NoError(t, err)
	assert.Equal(t, "yes", line)
}

func TestReadLine_OnlyFirstLineConsumed(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("first\nsecond\n"))

	line, err := readLine(r)
	require.NoError(t, err)
	assert.Equal(t, "first", line)

	line, err = readLine(r)
	require.NoError(t, err)
	assert.Equal(t, "second", line)
}

func TestReadLine_EOFWithoutNewline(t *testing.T) {
	line, err := readLine(bufio.NewReader(strings.NewReader("no newline")))

	require.ErrorIs(t, err, io.EOF)
	assert.Equal(t, "no newline", line)
}

func TestReadLine_EmptyInput(t *testing.T) {
	line, err := readLine(bufio.NewReader(strings.NewReader("")))

	require.ErrorIs(t, err, io.EOF)
	assert.Empty(t, line)
}

func TestReadLine_ArrowKeysDiscarded(t *testing.T) {
	// Reproduces a real bug: pressing Right arrow twice while typing a URL
	// embedded raw escape bytes ("abcdefghi\x1b[C\x1b[C") into the answer,
	// which then failed url.Parse downstream. Arrow keys send VT100 escape
	// sequences (ESC '[' 'C'/'D') on every platform, regardless of
	// cooked/raw console mode, so they must be discarded rather than
	// appended.
	line, err := readLine(bufio.NewReader(strings.NewReader("abcdefghi\x1b[C\x1b[C\n")))

	require.NoError(t, err)
	assert.Equal(t, "abcdefghi", line)
}

func TestReadLine_EscapeSequenceMidLine(t *testing.T) {
	line, err := readLine(bufio.NewReader(strings.NewReader("abc\x1b[Ddef\n")))

	require.NoError(t, err)
	assert.Equal(t, "abcdef", line)
}

func TestReadLine_BareEscapeKeypress(t *testing.T) {
	// A bare Escape keypress (not part of a CSI sequence) has no '[' after
	// it; that next byte must be pushed back and processed normally rather
	// than silently swallowed.
	line, err := readLine(bufio.NewReader(strings.NewReader("ab\x1bcd\n")))

	require.NoError(t, err)
	assert.Equal(t, "abcd", line)
}

func TestIsNonInteractive(t *testing.T) {
	cases := map[string]bool{
		"":      false,
		"true":  true,
		"TRUE":  true,
		"True":  true,
		"1":     true,
		"yes":   true,
		"y":     true,
		"false": false,
		"0":     false,
		"no":    false,
		"foo":   false,
	}

	for value, want := range cases {
		t.Setenv(NonInteractiveEnv, value)
		assert.Equalf(t, want, IsNonInteractive(), "value=%q", value)
	}
}
