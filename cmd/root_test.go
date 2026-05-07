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

package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCaseInsensitiveCommands(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		shouldError bool
	}{
		{
			name:        "HELP uppercase",
			args:        []string{"HELP"},
			shouldError: false,
		},
		{
			name:        "help lowercase",
			args:        []string{"help"},
			shouldError: false,
		},
		{
			name:        "Help mixed case",
			args:        []string{"Help"},
			shouldError: false,
		},
		{
			name:        "SELF uppercase",
			args:        []string{"SELF"},
			shouldError: false,
		},
		{
			name:        "self lowercase",
			args:        []string{"self"},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new root command for each test to ensure isolation
			cmd := RootCmd

			// Capture output
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			// Set the args
			cmd.SetArgs(tt.args)

			// Execute the command
			err := cmd.Execute()

			if tt.shouldError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestVersionFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "--version", args: []string{"--version"}},
		{name: "-V", args: []string{"-V"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := RootCmd
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			require.NoError(t, err)

			output := buf.String()
			assert.NotEmpty(t, output, "Version output should not be empty")
		})
	}
}

func TestWorkloadCommandNotPresentByDefault(t *testing.T) {
	// Verify that workload command is not present by default (feature not enabled).
	// The feature gating happens during init(), so this tests the actual state.
	cmd := RootCmd

	var found bool

	for _, subCmd := range cmd.Commands() {
		if subCmd.Name() == "workload" {
			found = true
			break
		}
	}

	assert.False(t, found, "workload command should not be present when feature gate is not enabled")
}
