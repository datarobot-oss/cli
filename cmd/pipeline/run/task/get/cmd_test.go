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

package get

import (
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/datarobot/cli/cmd/pipeline/internal/testutil"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runCmd(t *testing.T, args ...string) error {
	t.Helper()

	cmd := Cmd()
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil

	return cmd.Execute()
}

func TestCmd_RejectsMissingPipeline(t *testing.T) {
	err := runCmd(t, "--run", "d-1", "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline")
}

func TestCmd_RejectsMissingRun(t *testing.T) {
	err := runCmd(t, "--pipeline", "p-1", "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "run")
}

func TestCmd_RequiresPositionalArg(t *testing.T) {
	err := runCmd(t, "--pipeline", "p-1", "--run", "d-1")
	require.Error(t, err)
}

func TestCmd_RejectsNonIntegerTaskID(t *testing.T) {
	err := runCmd(t, "--pipeline", "p-1", "--run", "d-1", "notanumber")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid task-id")
}

func TestCmd_RejectsZeroTaskID(t *testing.T) {
	err := runCmd(t, "--pipeline", "p-1", "--run", "d-1", "0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid task-id")
}

func TestParseTaskID_Valid(t *testing.T) {
	id, err := parseTaskID("3")
	require.NoError(t, err)
	assert.Equal(t, 3, id)
}

func TestParseTaskID_RejectsZero(t *testing.T) {
	_, err := parseTaskID("0")
	require.Error(t, err)
}

func TestParseTaskID_RejectsNonInteger(t *testing.T) {
	_, err := parseTaskID("abc")
	require.Error(t, err)
}

func TestCmd_HasExpectedFlags(t *testing.T) {
	cmd := Cmd()

	for _, name := range []string{"pipeline", "run", "output-format"} {
		assert.NotNilf(t, cmd.Flags().Lookup(name), "expected --%s flag", name)
	}
}

func TestHandleNotFound_TextFormat_404_PrintsMessageAndReturnsNil(t *testing.T) {
	httpErr := &drapi.HTTPError{StatusCode: http.StatusNotFound, URL: "x"}

	output := testutil.CaptureStdout(t, func() {
		err := handleNotFound(httpErr, "1", outputformat.OutputFormatText)
		assert.NoError(t, err)
	})

	assert.Contains(t, output, "No task execution found with id: 1")
}

func TestHandleNotFound_JSONFormat_404_ReturnsErrorAndKeepsStdoutClean(t *testing.T) {
	httpErr := &drapi.HTTPError{StatusCode: http.StatusNotFound, URL: "x"}

	output := testutil.CaptureStdout(t, func() {
		err := handleNotFound(httpErr, "1", outputformat.OutputFormatJSON)
		require.Error(t, err)
		assert.Same(t, httpErr, err)
	})

	assert.Empty(t, output, "JSON mode must not write friendly message to stdout")
}

func TestHandleNotFound_JSONFormat_NonNotFound_Propagates(t *testing.T) {
	other := &drapi.HTTPError{StatusCode: http.StatusInternalServerError, URL: "x"}

	err := handleNotFound(other, "1", outputformat.OutputFormatJSON)
	require.Error(t, err)
	assert.Same(t, other, err)
}

func TestHandleNotFound_NonHTTPError_Propagates(t *testing.T) {
	plain := errors.New("network unreachable")

	err := handleNotFound(plain, "1", outputformat.OutputFormatJSON)
	require.Error(t, err)
	assert.Equal(t, plain, err)
}
