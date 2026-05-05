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

package syncc

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runWithFlags wires up Cmd() with PreRunE disabled (so tests don't go
// through auth) and the given flag set. Args other than the project
// directory are passed through. Returns the captured stdout/stderr and
// the error from cmd.Execute.
func runWithFlags(t *testing.T, flags map[string]string, extraArgs ...string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()

	cmd := Cmd()
	cmd.PreRunE = nil

	for k, v := range flags {
		require.NoError(t, cmd.Flags().Set(k, v))
	}

	cmd.SetArgs(extraArgs)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err := cmd.Execute()

	return cmd, stdout, stderr, err
}

// TestCmd_NotLinked confirms the command refuses to run when the
// target directory has no .wapi/, with the expected hint pointing at
// `code init`. Mirrors the spec's "not linked" preflight error.
func TestCmd_NotLinked(t *testing.T) {
	dir := t.TempDir() // no .wapi/ created

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true"}

	_, _, _, err := runWithFlags(t, flags)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not linked")
}

// TestCmd_DryRunDiffMutuallyExclusive confirms cobra's
// MarkFlagsMutuallyExclusive wiring is in place; both flags exit at
// the end of phase 4 so combining them has no meaning.
func TestCmd_DryRunDiffMutuallyExclusive(t *testing.T) {
	dir := t.TempDir()

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true", "diff": "true"}

	_, _, _, err := runWithFlags(t, flags)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "[dry-run diff]")
}
