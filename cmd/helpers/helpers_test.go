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

package helpers

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfirm_PrintsPrompt(t *testing.T) {
	var out bytes.Buffer

	_, _ = Confirm(&out, strings.NewReader("y\n"), "Install now? (y/n): ")

	assert.Equal(t, "Install now? (y/n): ", out.String())
}

func TestConfirm_AcceptsY(t *testing.T) {
	ok, err := Confirm(&bytes.Buffer{}, strings.NewReader("y\n"), "")

	require.NoError(t, err)
	assert.True(t, ok)
}

func TestConfirm_AcceptsYes(t *testing.T) {
	ok, err := Confirm(&bytes.Buffer{}, strings.NewReader("yes\n"), "")

	require.NoError(t, err)
	assert.True(t, ok)
}

func TestConfirm_AcceptsCaseInsensitive(t *testing.T) {
	for _, input := range []string{"Y\n", "YES\n", "Yes\n"} {
		ok, err := Confirm(&bytes.Buffer{}, strings.NewReader(input), "")

		require.NoError(t, err)
		assert.True(t, ok, "input %q should be accepted", input)
	}
}

func TestConfirm_RejectsN(t *testing.T) {
	ok, err := Confirm(&bytes.Buffer{}, strings.NewReader("n\n"), "")

	require.NoError(t, err)
	assert.False(t, ok)
}

func TestConfirm_RejectsArbitraryInput(t *testing.T) {
	ok, err := Confirm(&bytes.Buffer{}, strings.NewReader("maybe\n"), "")

	require.NoError(t, err)
	assert.False(t, ok)
}

func TestConfirm_EOFReturnsError(t *testing.T) {
	ok, err := Confirm(&bytes.Buffer{}, strings.NewReader(""), "")

	require.Error(t, err)
	assert.False(t, ok)
}
