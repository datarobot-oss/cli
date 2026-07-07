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

package tools

import (
	"bytes"
	"os"
	"testing"

	"github.com/datarobot/cli/internal/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureLog redirects os.Stderr to a pipe, reinitializes the logger, runs fn,
// then returns everything written to the logger during fn's execution.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	origStderr := os.Stderr
	os.Stderr = w

	log.StartStderr()

	fn()

	w.Close()

	os.Stderr = origStderr

	t.Cleanup(log.StopStderr)

	var buf bytes.Buffer

	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	r.Close()

	return buf.String()
}

func validPrerequisite() Prerequisite {
	return Prerequisite{
		Name:           "Python",
		MinimumVersion: "3.9.0",
		Command:        "python3 --version",
		URL:            "https://python.org",
		Install:        InstallCommands{MacOS: "brew install python", Linux: "apt install python3"},
	}
}

func TestValidatePrerequisite_ValidEntry(t *testing.T) {
	violations := validatePrerequisite("python", validPrerequisite())

	assert.Empty(t, violations)
}

func TestValidatePrerequisite_RequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Prerequisite)
		wantMsg string
	}{
		{
			name:    "missing name",
			mutate:  func(p *Prerequisite) { p.Name = "" },
			wantMsg: "'name' is required",
		},
		{
			name:    "missing minimum-version",
			mutate:  func(p *Prerequisite) { p.MinimumVersion = "" },
			wantMsg: "'minimum-version' is required",
		},
		{
			name:    "missing command",
			mutate:  func(p *Prerequisite) { p.Command = "" },
			wantMsg: "'command' is required",
		},
		{
			name:    "missing url",
			mutate:  func(p *Prerequisite) { p.URL = "" },
			wantMsg: "'url' is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := validPrerequisite()
			tt.mutate(&p)

			var violations []string

			output := captureLog(t, func() {
				violations = validatePrerequisite("tool", p)
			})

			assert.Contains(t, output, "WARN")
			assert.Contains(t, output, "[tool]")

			found := false

			for _, v := range violations {
				if assert.Contains(t, v, tt.wantMsg) {
					found = true

					break
				}
			}

			assert.True(t, found, "expected violation containing %q", tt.wantMsg)
		})
	}
}

func TestValidatePrerequisite_InvalidSemver(t *testing.T) {
	p := validPrerequisite()
	p.MinimumVersion = "not-a-version"

	var violations []string

	output := captureLog(t, func() {
		violations = validatePrerequisite("python", p)
	})

	assert.Contains(t, output, "WARN")
	assert.Contains(t, output, "[python]")
	assert.Contains(t, output, "not a valid semantic version")

	assert.NotEmpty(t, violations)
	assert.Contains(t, violations[0], "not a valid semantic version")
}

func TestValidatePrerequisite_InstallMacOSRequired(t *testing.T) {
	p := validPrerequisite()
	p.Install.MacOS = ""

	var violations []string

	output := captureLog(t, func() {
		violations = validatePrerequisite("tool", p)
	})

	assert.Contains(t, output, "WARN")
	assert.NotEmpty(t, violations)
	assert.Contains(t, violations[0], "macos")
}

func TestValidatePrerequisite_InstallLinuxRequired(t *testing.T) {
	p := validPrerequisite()
	p.Install.Linux = ""

	var violations []string

	output := captureLog(t, func() {
		violations = validatePrerequisite("tool", p)
	})

	assert.Contains(t, output, "WARN")
	assert.NotEmpty(t, violations)
	assert.Contains(t, violations[0], "linux")
}

func TestValidatePrerequisite_WindowsOptional(t *testing.T) {
	p := validPrerequisite()
	p.Install.Windows = ""

	output := captureLog(t, func() {
		violations := validatePrerequisite("tool", p)
		assert.Empty(t, violations)
	})

	assert.Empty(t, output)
}

func TestValidatePrerequisite_ViolationsLoggedAsWarn(t *testing.T) {
	p := validPrerequisite()
	p.Name = ""

	output := captureLog(t, func() {
		validatePrerequisite("tool", p)
	})

	assert.Contains(t, output, "WARN")
	assert.NotContains(t, output, "ERRO")
}

func TestValidatePrerequisite_MessageContainsKey(t *testing.T) {
	p := validPrerequisite()
	p.Name = ""

	violations := validatePrerequisite("my-tool", p)

	assert.NotEmpty(t, violations)
	assert.Contains(t, violations[0], "[my-tool]")
}
