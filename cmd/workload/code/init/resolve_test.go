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

package initcmd

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveDir(t *testing.T) {
	cases := []struct {
		name             string
		flag             string
		yes              bool
		tty              bool
		promptReturns    string
		promptErr        error
		wantDir          string
		wantPromptCalled bool
	}{
		{name: "FlagWins", flag: "/tmp/x", yes: false, tty: true, wantDir: "/tmp/x"},
		{name: "FlagWinsEvenWithYes", flag: "/tmp/x", yes: true, tty: false, wantDir: "/tmp/x"},
		{name: "YesUsesDot", flag: "", yes: true, tty: true, wantDir: "."},
		{name: "NonTTYUsesDot", flag: "", yes: false, tty: false, wantDir: "."},
		{name: "TTYPromptDefault", flag: "", yes: false, tty: true, promptReturns: ".", wantDir: ".", wantPromptCalled: true},
		{name: "TTYPromptCustom", flag: "", yes: false, tty: true, promptReturns: "./svc", wantDir: "./svc", wantPromptCalled: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false

			prompt := func(label, def string) (string, error) {
				called = true

				assert.Equal(t, "Initialize directory", label)
				assert.Equal(t, ".", def)

				return tc.promptReturns, tc.promptErr
			}

			got, err := resolveDir(tc.flag, tc.yes, tc.tty, prompt)
			require.NoError(t, err)
			assert.Equal(t, tc.wantDir, got)
			assert.Equal(t, tc.wantPromptCalled, called)
		})
	}
}

func TestResolveDir_PromptError(t *testing.T) {
	prompt := func(_, _ string) (string, error) {
		return "", errors.New("read failed")
	}

	got, err := resolveDir("", false, true, prompt)
	require.Error(t, err)
	assert.Empty(t, got)
}

func TestResolveArtifactID(t *testing.T) {
	cases := []struct {
		name             string
		args             []string
		yes              bool
		tty              bool
		promptReturns    string
		wantID           string
		wantErrSubstring string
		wantPromptCalled bool
	}{
		{name: "Positional", args: []string{"art-abc-123"}, wantID: "art-abc-123"},
		{name: "PositionalEvenWithYes", args: []string{"art-abc-123"}, yes: true, wantID: "art-abc-123"},
		{name: "YesNoIDErrors", args: []string{}, yes: true, tty: true, wantErrSubstring: "artifact ID is required when using --yes"},
		{name: "NonTTYNoIDErrors", args: []string{}, yes: false, tty: false, wantErrSubstring: "artifact ID is required (no TTY for prompting)"},
		{name: "TTYPrompts", args: []string{}, yes: false, tty: true, promptReturns: "art-xyz-789", wantID: "art-xyz-789", wantPromptCalled: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false

			prompt := func(label string) (string, error) {
				called = true

				assert.Equal(t, "Artifact ID", label)

				return tc.promptReturns, nil
			}

			got, err := resolveArtifactID(tc.args, tc.yes, tc.tty, prompt)

			if tc.wantErrSubstring != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrSubstring)
				assert.Empty(t, got)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantID, got)
			assert.Equal(t, tc.wantPromptCalled, called)
		})
	}
}
