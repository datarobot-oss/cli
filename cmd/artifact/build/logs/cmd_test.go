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

package logs

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCmd_RequiresAtLeastOneArg(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.Error(t, err)
}

func TestCmd_RejectsThirdArg(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"art-1", "b-1", "extra"})

	err := cmd.Execute()
	require.Error(t, err)
}

func TestCmd_InvalidLevel(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"art-1", "b-1", "--level", "trace"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --level")
}

// Lines that exist below --level are reported as such, not as "no logs".
func TestCmd_SaysWhenTheLinesSitBelowTheLevel(t *testing.T) {
	prev := getLogsFn
	getLogsFn = func(string, string) ([]workload.BuildLogEntry, error) {
		return []workload.BuildLogEntry{{Levelname: "DEBUG", Message: "pip install"}, {Levelname: "DEBUG", Message: "done"}}, nil
	}

	t.Cleanup(func() { getLogsFn = prev })

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"art-1", "b-1"})

	var stderr bytes.Buffer

	cmd.SetErr(&stderr)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "No logs at level info or above; 2 below it. Use --level debug to see them.\n", stderr.String())
}

// Under JSON the same situation prints an empty list on stdout and nothing
// on stderr, so `| jq` parses.
func TestCmd_JSONPrintsAnEmptyListWhenTheLinesSitBelowTheLevel(t *testing.T) {
	prev := getLogsFn
	getLogsFn = func(string, string) ([]workload.BuildLogEntry, error) {
		return []workload.BuildLogEntry{{Levelname: "DEBUG", Message: "pip install"}}, nil
	}

	t.Cleanup(func() { getLogsFn = prev })

	stdout := captureStdout(t)

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"art-1", "b-1", "--output-format", "json"})

	var stderr bytes.Buffer

	cmd.SetErr(&stderr)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "[]", strings.TrimSpace(stdout()))
	assert.Empty(t, stderr.String())
}

// captureStdout redirects the process's stdout until the returned function
// is called, which hands back what was written.
func captureStdout(t *testing.T) func() string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	prev := os.Stdout
	os.Stdout = w

	t.Cleanup(func() { os.Stdout = prev })

	return func() string {
		os.Stdout = prev

		require.NoError(t, w.Close())

		out, err := io.ReadAll(r)
		require.NoError(t, err)

		return string(out)
	}
}

func TestCmd_InvalidOutputFormat(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"art-1", "b-1", "--output-format", "yaml"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid output format")
}
