// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

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
			name:        "VERSION uppercase",
			args:        []string{"VERSION"},
			shouldError: false,
		},
		{
			name:        "version lowercase",
			args:        []string{"version"},
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
