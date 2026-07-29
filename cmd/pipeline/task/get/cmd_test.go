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

func TestCmd_Name(t *testing.T) {
	assert.Equal(t, "get", Cmd().Name())
}

func TestCmd_RejectsMissingPipeline(t *testing.T) {
	err := runCmd(t, "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline")
}

func TestCmd_RequiresPositional(t *testing.T) {
	err := runCmd(t, "--pipeline", "p")
	require.Error(t, err)
}

func TestCmd_RejectsNonNumericTaskID(t *testing.T) {
	err := runCmd(t, "--pipeline", "p", "abc123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task-id must be a positive number")
}

func TestCmd_RejectsZeroTaskID(t *testing.T) {
	err := runCmd(t, "--pipeline", "p", "0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task-id must be a positive number")
}

func TestCmd_RejectsBadScopeCombo(t *testing.T) {
	err := runCmd(t, "--pipeline", "p", "--scope", "locked", "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--version")
}

func TestCmd_RejectsInvalidOutputFormat(t *testing.T) {
	err := runCmd(t, "--pipeline", "p", "--output-format", "yaml", "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid output format")
}

func TestHandleTaskNotFoundError_TextFormat_404_PrintsMessageAndReturnsNil(t *testing.T) {
	httpErr := &drapi.HTTPError{StatusCode: http.StatusNotFound, URL: "x"}

	output := testutil.CaptureStdout(t, func() {
		err := handleTaskNotFoundError(httpErr, "1", outputformat.OutputFormatText)
		assert.NoError(t, err)
	})

	assert.Contains(t, output, "Task not found: 1")
}

func TestHandleTaskNotFoundError_JSONFormat_404_ReturnsErrorAndKeepsStdoutClean(t *testing.T) {
	httpErr := &drapi.HTTPError{StatusCode: http.StatusNotFound, URL: "x"}

	output := testutil.CaptureStdout(t, func() {
		err := handleTaskNotFoundError(httpErr, "1", outputformat.OutputFormatJSON)
		require.Error(t, err)
		assert.Same(t, httpErr, err)
	})

	assert.Empty(t, output, "JSON mode must not write friendly message to stdout")
}

func TestHandleTaskNotFoundError_JSONFormat_NonNotFound_Propagates(t *testing.T) {
	other := &drapi.HTTPError{StatusCode: http.StatusInternalServerError, URL: "x"}

	err := handleTaskNotFoundError(other, "1", outputformat.OutputFormatJSON)
	require.Error(t, err)
	assert.Same(t, other, err)
}

func TestHandleTaskNotFoundError_NonHTTPError_Propagates(t *testing.T) {
	plain := errors.New("network unreachable")

	err := handleTaskNotFoundError(plain, "1", outputformat.OutputFormatJSON)
	require.Error(t, err)
	assert.Equal(t, plain, err)
}
