// Copyright 2025 DataRobot, Inc. and its affiliates.
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
				assert.NoError(t, err)
			}
		})
	}
}
