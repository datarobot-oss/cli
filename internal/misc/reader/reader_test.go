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
	"errors"
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

	require.Error(t, err)
	assert.True(t, errors.Is(err, io.EOF))
	assert.Equal(t, "no newline", line)
}

func TestReadLine_EmptyInput(t *testing.T) {
	line, err := readLine(bufio.NewReader(strings.NewReader("")))

	require.Error(t, err)
	assert.True(t, errors.Is(err, io.EOF))
	assert.Equal(t, "", line)
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
